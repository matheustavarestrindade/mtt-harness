package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestResponsesContinuationRequiresUnchangedInputAndRequestProperties(test *testing.T) {
	mutations := map[string]func(*atom.Request){
		"history edited": func(request *atom.Request) {
			request.Messages[1].Content = []atom.Content{{Type: atom.Text, Text: "Changed source"}}
		},
		"history removed": func(request *atom.Request) { request.Messages = append(request.Messages[:1], request.Messages[2:]...) },
		"history reordered": func(request *atom.Request) {
			request.Messages[1], request.Messages[2] = request.Messages[2], request.Messages[1]
		},
		"instructions changed": func(request *atom.Request) {
			request.Messages[0].Content = []atom.Content{{Type: atom.Text, Text: "Changed instructions"}}
		},
		"model changed":  func(request *atom.Request) { request.Model = "other-model" },
		"effort changed": func(request *atom.Request) { request.ReasoningEffort = "high" },
		"tools changed": func(request *atom.Request) {
			request.Tools = []atom.ToolSpec{{Name: "new-tool", InputSchema: atom.Schema{JSON: json.RawMessage(`{"type":"object"}`)}}}
		},
		"params changed": func(request *atom.Request) { request.Params = map[string]any{"temperature": 0.2} },
		"overlay moved": func(request *atom.Request) {
			request.Messages = append([]atom.Message{request.Messages[0], {Role: atom.RoleSystem, InContext: true, Content: []atom.Content{{Type: atom.Text, Text: "New reminder"}}}}, request.Messages[1:]...)
		},
	}
	for name, mutate := range mutations {
		test.Run(name, func(test *testing.T) {
			operationContext := transportTestContext(test, "workspace", "session")
			requests := make(chan map[string]any, 2)
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				connection, operationError := websocket.Accept(responseWriter, request, nil)
				if operationError != nil {
					return
				}
				defer connection.CloseNow()
				for index := 0; index < 2; index++ {
					payload, operationError := readWebSocketRequest(operationContext, connection)
					if operationError != nil {
						return
					}
					requests <- payload
					if completeWebSocketResponse(operationContext, connection, fmt.Sprint(index), "Ready") != nil {
						return
					}
				}
			}))
			test.Cleanup(server.Close)
			modelProvider := New(atom.ProviderSpec{Name: "compatible", Protocol: "responses", APIURL: server.URL, Transport: "websocket"})
			test.Cleanup(func() { modelProvider.Close() })
			modelProvider.SetModels([]atom.ModelInfo{{ID: "transport-model", Reasoning: true, ReasoningEfforts: []string{"low", "high"}}, {ID: "other-model", Reasoning: true, ReasoningEfforts: []string{"low", "high"}}})
			request := atom.Request{Model: "transport-model", ReasoningEffort: "low", Messages: []atom.Message{{Role: atom.RoleSystem, Content: []atom.Content{{Type: atom.Text, Text: "Instructions"}}}, {Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Source"}}}, {Role: atom.RoleRuntime, Content: []atom.Content{{Type: atom.Text, Text: "Observed output"}}}}}
			stream, operationError := modelProvider.Stream(operationContext, request)
			testutil.RequireNoError(test, operationError)
			message, operationError := collectTransportMessage(operationContext, stream)
			testutil.RequireNoError(test, operationError)
			request.Messages = append(request.Messages, message, atom.Message{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Next"}}})
			mutate(&request)
			stream, operationError = modelProvider.Stream(operationContext, request)
			testutil.RequireNoError(test, operationError)
			_, operationError = collectTransportMessage(operationContext, stream)
			testutil.RequireNoError(test, operationError)
			<-requests
			current := <-requests
			if current["previous_response_id"] != nil {
				test.Fatalf("changed request reused old context: %v", current)
			}
			input, _, operationError := encodeResponsesAPIInput(request.Messages, modelProvider.Name())
			testutil.RequireNoError(test, operationError)
			expected, _ := json.Marshal(input)
			actual, _ := json.Marshal(current["input"])
			if string(actual) != string(expected) {
				test.Fatalf("full context changed: %s != %s", actual, expected)
			}
		})
	}
}

func TestResponsesWebSocketSeparatesWorkspacesCredentialsAndWorkers(test *testing.T) {
	operationContext := transportTestContext(test, "workspace-one", "same-session")
	var upgrades atomic.Int32
	var credential atomic.Value
	credential.Store("first-token")
	requests := make(chan map[string]any, 6)
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		connection, operationError := websocket.Accept(responseWriter, request, nil)
		if operationError != nil {
			return
		}
		defer connection.CloseNow()
		upgrades.Add(1)
		for {
			payload, operationError := readWebSocketRequest(operationContext, connection)
			if operationError != nil {
				return
			}
			requests <- payload
			if completeWebSocketResponse(operationContext, connection, "response", "Ready") != nil {
				return
			}
		}
	}))
	test.Cleanup(server.Close)
	modelProvider := New(atom.ProviderSpec{Name: "compatible", Protocol: "responses", APIURL: server.URL, Transport: "websocket", Authentication: "api_key"})
	test.Cleanup(func() { modelProvider.Close() })
	modelProvider.SetKeyResolver(func(context.Context, string, string) (string, error) { return credential.Load().(string), nil })
	request := atom.Request{Model: "transport-model", Messages: []atom.Message{{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Source"}}}}}
	run := func(currentContext context.Context) atom.Message {
		stream, operationError := modelProvider.Stream(currentContext, request)
		testutil.RequireNoError(test, operationError)
		message, operationError := collectTransportMessage(currentContext, stream)
		testutil.RequireNoError(test, operationError)
		if (<-requests)["previous_response_id"] != nil {
			test.Fatal("transport leaked another owner's continuation")
		}
		return message
	}
	message := run(operationContext)
	request.Messages = append(request.Messages, message, atom.Message{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Next"}}})
	credential.Store("refreshed-token")
	run(operationContext)
	run(harness.WithSession(operationContext, atom.Session{InstanceID: "workspace-two", ID: "same-session"}))
	workerContext := harness.WithSession(operationContext, atom.Session{InstanceID: "workspace-one"})
	run(workerContext)
	run(workerContext)
	if upgrades.Load() != 5 {
		test.Fatalf("credentials/workspace/worker scopes shared sockets: %d", upgrades.Load())
	}
	modelProvider.websockets.evictIdleConnections(time.Now().Add(responsesConnectionIdleLimit + time.Second))
	modelProvider.websockets.mutex.Lock()
	remaining := len(modelProvider.websockets.entries)
	modelProvider.websockets.mutex.Unlock()
	if remaining != 0 {
		test.Fatalf("idle sockets were retained: %d", remaining)
	}
}

func TestResponsesContinuationIncludesReasoningAndToolOutput(test *testing.T) {
	modelProvider := New(atom.ProviderSpec{Name: "compatible", Protocol: "responses"})
	defer modelProvider.Close()
	request := atom.Request{Model: "transport-model", Messages: []atom.Message{{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Source"}}}}}
	payload, operationError := modelProvider.buildResponsesPayload(request)
	testutil.RequireNoError(test, operationError)
	completion := &responseEvent{}
	completion.Response.ID = "response"
	completion.Response.Output = []json.RawMessage{json.RawMessage(`{"type":"reasoning","id":"reason","encrypted_content":"opaque","summary":[]}`), json.RawMessage(`{"type":"function_call","call_id":"call","name":"inspect","arguments":"{}","status":"completed"}`)}
	baseline := captureResponsesContinuation(payload, completion, modelProvider.Name())
	request.Messages = append(request.Messages, atom.Message{Role: atom.RoleAssistant, ProviderState: &atom.ProviderState{Provider: "compatible", Reasoning: completion.Response.Output[:1]}, ToolCalls: []atom.ToolCall{{ID: "call", Name: "inspect", Input: json.RawMessage(`{}`)}}}, atom.Message{Role: atom.RoleTool, ToolCallID: "call", Content: []atom.Content{{Type: atom.Text, Text: "Observed"}}})
	next, operationError := modelProvider.buildResponsesPayload(request)
	testutil.RequireNoError(test, operationError)
	delta, valid := baseline.incrementalInput(next)
	if !valid || len(delta) != 1 || delta[0].(map[string]any)["type"] != "function_call_output" {
		test.Fatalf("tool continuation was not incremental: %v %v", valid, delta)
	}
	request.Messages[1].ProviderState = &atom.ProviderState{Provider: "different-provider", Reasoning: completion.Response.Output[:1]}
	next, operationError = modelProvider.buildResponsesPayload(request)
	testutil.RequireNoError(test, operationError)
	if _, valid := baseline.incrementalInput(next); valid {
		test.Fatal("foreign-provider reasoning reused the old context")
	}
}
