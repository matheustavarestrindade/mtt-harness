package loop_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
	"github.com/matheustavarestrindade/mtt-harness/internal/tools"
)

type reasoningTestProvider struct{ *provider.Test }

func (modelProvider reasoningTestProvider) Models() []atom.ModelInfo {
	models := modelProvider.Test.Models()
	models[0].Reasoning = true
	models[0].ReasoningEfforts = []string{"low", "high"}
	return models
}

func TestSavedEffortChangesAtRequestBoundariesAndPublicThinkingPersists(test *testing.T) {
	testStack := newStack(test,
		[]atom.ResponsePart{
			{Reasoning: "Initial plan"},
			{ProviderState: &atom.ProviderState{Provider: "test", Reasoning: []json.RawMessage{json.RawMessage(`{"type":"reasoning","encrypted_content":"private-ciphertext"}`)}}},
			{ToolCall: &atom.ToolCall{ID: "discover", Name: "search_tool", Input: []byte(`{"category":"file"}`)}},
		},
		[]atom.ResponsePart{{Reasoning: "Final check"}, {Text: "done"}},
	)
	testStack.harnessRuntime.Provider(reasoningTestProvider{testStack.provider})
	testStack.bus.SetRecorder(testStack.database.Events().Record)
	testutil.RequireNoError(test, testStack.registry.Add(tools.NewSearch(testStack.registry)))
	session := testStack.instance(test, 2)
	lowSelection := atom.SessionModelSelection{Model: session.Model, ReasoningEffort: "low"}
	testutil.RequireNoError(test, testStack.database.Sessions().SetModelSelection(context.Background(), session.ID, session.ModelSelection(), lowSelection))
	harness.Pipe(testStack.harnessRuntime, atom.StageToolResult, func(operationContext context.Context, result atom.ToolResult) (atom.ToolResult, error) {
		return result, testStack.database.Sessions().SetModelSelection(operationContext, session.ID, lowSelection, atom.SessionModelSelection{Model: session.Model, ReasoningEffort: "high"})
	})
	testStack.user(test, session, "continue after the tool")
	testutil.RequireNoError(test, testStack.loop.Run(context.Background(), session))
	if len(testStack.provider.Requests) != 2 || testStack.provider.Requests[0].ReasoningEffort != "low" || testStack.provider.Requests[1].ReasoningEffort != "high" {
		test.Fatalf("stored selection did not reach request boundaries: %+v", testStack.provider.Requests)
	}
	messages, operationError := testStack.database.Sessions().Messages(context.Background(), session.ID)
	testutil.RequireNoError(test, operationError)
	var reasoning []string
	for _, message := range messages {
		if message.Role == atom.RoleAssistant {
			reasoning = append(reasoning, message.Reasoning)
		}
	}
	if strings.Join(reasoning, ";") != "Initial plan;Final check" {
		test.Fatalf("reasoning was lost or mixed into answers: %v", reasoning)
	}
	public, operationError := json.Marshal(messages)
	testutil.RequireNoError(test, operationError)
	if strings.Contains(string(public), "private-ciphertext") {
		test.Fatal("public messages leaked continuation state")
	}
	events, operationError := testStack.database.Events().Since(context.Background(), session.InstanceID, 0)
	testutil.RequireNoError(test, operationError)
	seenThinking := false
	for _, event := range events {
		if strings.Contains(string(event.Payload), "private-ciphertext") {
			test.Fatal("events leaked continuation state")
		}
		if event.Name != atom.EventModelChunk {
			continue
		}
		var payload map[string]string
		testutil.RequireNoError(test, json.Unmarshal(event.Payload, &payload))
		if payload["reasoning"] == "Initial plan" && payload["message_id"] != "" {
			seenThinking = true
		}
	}
	if !seenThinking {
		test.Fatal("thinking was not emitted with its message identity")
	}
}
