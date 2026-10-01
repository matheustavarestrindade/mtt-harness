package loop_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/startprompt"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

func TestStartupPromptRemainsSingleAcrossToolsAndSessionTurns(test *testing.T) {
	template, operationError := startprompt.Parse("START {session_id} {workspace} {agent_depth}\n{tool_list}\n{agent_info}")
	testutil.RequireNoError(test, operationError)
	testStack := newStackWithStartPrompt(test, template, provider.Call("fake", `{}`), provider.Text("done"))
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "fake"}))
	testutil.RequireNoError(test, testStack.registry.Add(&tools.Agent{RunTask: testStack.loop.RunAgentTask}))
	contextPasses := 0
	harness.Pipe(testStack.harnessRuntime, atom.StageContextBuild, func(operationContext context.Context, messages []atom.Message) ([]atom.Message, error) {
		contextPasses++
		if len(messages) == 0 || messages[0].Role != atom.RoleSystem {
			test.Fatal("startup prompt was not supplied to context middleware")
		}
		return messages, nil
	})
	session := testStack.instance(test, 2)
	testStack.user(test, session, "run a tool")
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
	testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "late-tool"}))
	testutil.RequireNoError(test, testStack.database.Sessions().Append(context.Background(), atom.Message{ID: "second-user", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "continue"}}}))
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
	if len(testStack.provider.Requests) != 3 || contextPasses != 3 {
		test.Fatal("missing tool-round or subsequent-turn request")
	}
	for requestIndex, request := range testStack.provider.Requests {
		systemMessages := 0
		for _, message := range request.Messages {
			if message.Role == atom.RoleSystem {
				systemMessages++
			}
		}
		prompt := request.Messages[0].Content[0].Text
		if systemMessages != 1 || !strings.Contains(prompt, string(session.ID)) || !strings.Contains(prompt, "Available models:") || !strings.Contains(prompt, `"default": "test/test-model"`) {
			test.Fatalf("startup context or live agent metadata is wrong: %s", prompt)
		}
		if strings.Contains(prompt, "late-tool") != (requestIndex == 2) {
			test.Fatal("tool list did not follow the current registry")
		}
		for _, definition := range request.Tools {
			if definition.Name == "agent" {
				test.Fatal("rendering agent_info made the agent tool callable")
			}
		}
	}
	messages, operationError := testStack.database.Sessions().Messages(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	for _, message := range messages {
		if message.Role == atom.RoleSystem {
			test.Fatal("request-only startup prompt was persisted in conversation history")
		}
	}
}

func TestChildAgentsGetTheirOwnStartupContext(test *testing.T) {
	template, operationError := startprompt.Parse("session={session_id};depth={agent_depth};model={model};workspace={workspace}")
	testutil.RequireNoError(test, operationError)
	testStack := newStackWithStartPrompt(test, template,
		provider.Call("agent", `{"task":"find the answer","model":"worker/worker-model"}`),
		provider.Text("done"),
	)
	childProvider := provider.NewTest("worker", provider.Call("finish", `{"result":"answer"}`))
	testStack.harnessRuntime.Provider(childProvider)
	testutil.RequireNoError(test, testStack.registry.Add(&tools.Agent{RunTask: testStack.loop.RunAgentTask}))
	testutil.RequireNoError(test, testStack.registry.Add(tools.Finish{}))
	session := testStack.instance(test, 2)
	testStack.user(test, session, "delegate")
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
	if len(testStack.provider.Requests) != 2 || len(childProvider.Requests) != 1 {
		test.Fatal("child or parent did not run")
	}
	parentPrompt := testStack.provider.Requests[0].Messages[0]
	childPrompt := childProvider.Requests[0].Messages[0]
	if parentPrompt.SessionID != session.ID || childPrompt.SessionID == session.ID || !strings.Contains(childPrompt.Content[0].Text, "depth=1") {
		test.Fatalf("child inherited the parent startup context: %+v", childPrompt)
	}
	instance, _ := testStack.instances.Get(session.InstanceID)
	if !strings.Contains(childPrompt.Content[0].Text, ";model=worker/worker-model;") || !strings.Contains(childPrompt.Content[0].Text, ";workspace="+instance.Spec().Workspace) {
		test.Fatalf("child model or workspace is incorrect: %+v", childPrompt)
	}
}

func TestStartupPromptErrorsAndBudgetStopOnlyTheTurn(test *testing.T) {
	for _, scenario := range []struct{ source, expectedError string }{
		{"{unregistered_info}", "not registered"},
		{strings.Repeat("large prompt ", 12000), "context budget"},
	} {
		template, operationError := startprompt.Parse(scenario.source)
		testutil.RequireNoError(test, operationError)
		testStack := newStackWithStartPrompt(test, template, provider.Text("done"))
		session := testStack.instance(test, 2)
		testStack.user(test, session, "hello")
		if operationError := testStack.loop.Run(context.Background(), session); operationError == nil || !strings.Contains(operationError.Error(), scenario.expectedError) {
			test.Fatalf("expected %q error, got %v", scenario.expectedError, operationError)
		}
		if len(testStack.provider.Requests) != 0 {
			test.Fatal("invalid prompt was sent to a provider")
		}
		if scenario.source == "{unregistered_info}" {
			testutil.RequireNoError(test, testStack.registry.Add(&fakeTool{name: "unregistered"}))
			testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
			if len(testStack.provider.Requests) != 1 {
				test.Fatal("session did not recover after the missing tool became available")
			}
		}
	}
}

func TestStartupPromptReachesProviderWireFormats(test *testing.T) {
	for _, configuration := range []struct{ protocol, authentication string }{{"chat_completions", "api_key"}, {"responses", "api_key"}, {"responses", "chatgpt"}} {
		test.Run(configuration.protocol+"/"+configuration.authentication, func(test *testing.T) {
			payloads := make(chan map[string]json.RawMessage, 1)
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				var payload map[string]json.RawMessage
				if operationError := json.NewDecoder(request.Body).Decode(&payload); operationError != nil {
					test.Error(operationError)
					http.Error(responseWriter, "invalid request", 400)
					return
				}
				payloads <- payload
				responseWriter.Header().Set("Content-Type", "text/event-stream")
				if configuration.protocol == "responses" {
					fmt.Fprint(responseWriter, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"done\"}]}]}}\n\n")
					return
				}
				fmt.Fprint(responseWriter, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			template, operationError := startprompt.Parse("Startup for {session_id}:\n{bash_info}")
			testutil.RequireNoError(test, operationError)
			testStack := newStackWithStartPrompt(test, template)
			testutil.RequireNoError(test, testStack.registry.Add(tools.NewBash(nil)))
			standardProvider := provider.New(atom.ProviderSpec{Name: "wire", Protocol: configuration.protocol, Authentication: configuration.authentication, APIURL: server.URL})
			standardProvider.SetKeyResolver(func(context.Context, string, string) (string, error) { return "fixture-key", nil })
			standardProvider.SetModels([]atom.ModelInfo{{ID: "wire-model", Tools: true}})
			testStack.harnessRuntime.Provider(standardProvider)
			session := testStack.instance(test, 2)
			session.Model = "wire/wire-model"
			testStack.user(test, session, "hello")
			testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
			payload := <-payloads
			var instructions string
			if configuration.protocol == "responses" {
				testutil.RequireNoError(test, json.Unmarshal(payload["instructions"], &instructions))
			} else {
				var messages []struct{ Role, Content string }
				testutil.RequireNoError(test, json.Unmarshal(payload["messages"], &messages))
				if len(messages) != 2 || messages[0].Role != "system" {
					test.Fatalf("startup message absent from wire payload: %s", payload["messages"])
				}
				instructions = messages[0].Content
			}
			if !strings.Contains(instructions, "Startup for "+string(session.ID)) || !strings.Contains(instructions, "milliseconds") || !strings.Contains(instructions, `"default": 0`) {
				test.Fatalf("startup instructions or tool schema were lost on the wire: %s", instructions)
			}
		})
	}
}
