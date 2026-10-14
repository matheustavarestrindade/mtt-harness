package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func transportTestContext(test *testing.T, workspaceID, sessionID string) context.Context {
	operationContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	test.Cleanup(cancel)
	return harness.WithSession(operationContext, atom.Session{InstanceID: workspaceID, ID: atom.SessionID(sessionID)})
}

func readWebSocketRequest(operationContext context.Context, connection *websocket.Conn) (map[string]any, error) {
	messageType, data, operationError := connection.Read(operationContext)
	if operationError != nil {
		return nil, operationError
	}
	if messageType != websocket.MessageText {
		return nil, fmt.Errorf("request is not a text frame")
	}
	var request map[string]any
	operationError = json.Unmarshal(data, &request)
	return request, operationError
}

func writeWebSocketEvent(operationContext context.Context, connection *websocket.Conn, event any) error {
	data, operationError := json.Marshal(event)
	if operationError != nil {
		return operationError
	}
	return connection.Write(operationContext, websocket.MessageText, data)
}

func completeWebSocketResponse(operationContext context.Context, connection *websocket.Conn, identifier, text string) error {
	return writeWebSocketEvent(operationContext, connection, map[string]any{"type": "response.completed", "response": map[string]any{"id": identifier, "output": []any{map[string]any{"type": "message", "role": "assistant", "id": "output-" + identifier, "status": "completed", "content": []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}}}}}, "usage": map[string]any{"input_tokens": 100, "output_tokens": 4}}})
}

func collectTransportMessage(operationContext context.Context, stream harness.Stream) (atom.Message, error) {
	message := atom.Message{Role: atom.RoleAssistant}
	var text strings.Builder
	for {
		part, operationError := stream.Recv(operationContext)
		text.WriteString(part.Text)
		if part.ToolCall != nil {
			message.ToolCalls = append(message.ToolCalls, *part.ToolCall)
		}
		if part.ProviderState != nil {
			message.ProviderState = part.ProviderState
		}
		if part.Usage != nil {
			message.Usage = part.Usage
		}
		if operationError != nil {
			if text.Len() > 0 {
				message.Content = []atom.Content{{Type: atom.Text, Text: text.String()}}
			}
			if errors.Is(operationError, io.EOF) {
				return message, nil
			}
			return message, operationError
		}
	}
}

func TestResponsesWebSocketReusesConnectionAndSendsOnlyNewInput(test *testing.T) {
	operationContext := transportTestContext(test, "workspace", "conversation")
	requests := make(chan map[string]any, 2)
	serverErrors := make(chan error, 2)
	var upgrades atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		upgrades.Add(1)
		if request.URL.Path != "/responses" || request.Header.Get("Authorization") != "Bearer private-key" || request.Header.Get("OpenAI-Beta") != "fixture-beta" || request.Header.Get("session_id") != "conversation" {
			serverErrors <- fmt.Errorf("unexpected handshake path/headers")
			http.Error(responseWriter, "bad handshake", 400)
			return
		}
		connection, operationError := websocket.Accept(responseWriter, request, &websocket.AcceptOptions{CompressionMode: websocket.CompressionContextTakeover})
		if operationError != nil {
			serverErrors <- operationError
			return
		}
		defer connection.CloseNow()
		connection.SetReadLimit(4 * 1024 * 1024)
		for index := 0; index < 2; index++ {
			payload, operationError := readWebSocketRequest(operationContext, connection)
			if operationError != nil {
				serverErrors <- operationError
				return
			}
			requests <- payload
			if operationError := completeWebSocketResponse(operationContext, connection, fmt.Sprintf("response-%d", index), "Ready"); operationError != nil {
				serverErrors <- operationError
				return
			}
		}
	}))
	test.Cleanup(server.Close)
	modelProvider := New(atom.ProviderSpec{Name: "compatible", Protocol: "responses", Authentication: "api_key", APIURL: server.URL, Transport: "websocket"})
	test.Cleanup(func() { testutil.RequireNoError(test, modelProvider.Close()) })
	modelProvider.SetKeyResolver(func(context.Context, string, string) (string, error) { return "private-key", nil })
	modelProvider.SetWebSocketHeaders(http.Header{"Openai-Beta": []string{"fixture-beta"}})
	request := atom.Request{Model: "transport-model", Messages: []atom.Message{{Role: atom.RoleSystem, Content: []atom.Content{{Type: atom.Text, Text: "Stable instructions"}}}, {Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: strings.Repeat("source ", 160000)}}}}, Params: map[string]any{"max_output_tokens": 200}}
	stream, operationError := modelProvider.Stream(operationContext, request)
	testutil.RequireNoError(test, operationError)
	message, operationError := collectTransportMessage(operationContext, stream)
	testutil.RequireNoError(test, operationError)
	request.Messages = append(request.Messages, message, atom.Message{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Next"}}})
	stream, operationError = modelProvider.Stream(operationContext, request)
	testutil.RequireNoError(test, operationError)
	_, operationError = collectTransportMessage(operationContext, stream)
	testutil.RequireNoError(test, operationError)
	first, second := <-requests, <-requests
	if upgrades.Load() != 1 || first["type"] != "response.create" || first["stream"] != nil || first["background"] != nil || first["store"] != false || first["instructions"] != "Stable instructions" {
		test.Fatalf("invalid initial WebSocket request: upgrades=%d properties=%v", upgrades.Load(), first["type"])
	}
	if second["previous_response_id"] != "response-0" || len(second["input"].([]any)) != 1 {
		test.Fatalf("continuation resent old input: %v", second)
	}
	if second["input"].([]any)[0].(map[string]any)["content"] != "Next" || second["max_output_tokens"] != float64(200) {
		test.Fatalf("continuation changed the request: %v", second)
	}
	firstBytes, _ := json.Marshal(first)
	secondBytes, _ := json.Marshal(second)
	if len(secondBytes) >= len(firstBytes)/100 {
		test.Fatalf("incremental input did not reduce transmitted bytes: %d -> %d", len(firstBytes), len(secondBytes))
	}
	select {
	case operationError := <-serverErrors:
		test.Fatal(operationError)
	default:
	}
}

func TestResponsesWebSocketStreamsToolsReasoningUsageAndPreset(test *testing.T) {
	operationContext := transportTestContext(test, "workspace", "tool-session")
	requests := make(chan map[string]any, 1)
	allowCompletion := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		connection, operationError := websocket.Accept(responseWriter, request, nil)
		if operationError != nil {
			return
		}
		defer connection.CloseNow()
		payload, operationError := readWebSocketRequest(operationContext, connection)
		if operationError != nil {
			return
		}
		requests <- payload
		for _, event := range []any{
			map[string]any{"type": "response.created", "response": map[string]any{"id": "tool-response"}},
			map[string]any{"type": "response.reasoning_summary_text.delta", "delta": "Inspecting"},
			map[string]any{"type": "response.output_item.done", "output_index": 1, "item": map[string]any{"type": "function_call", "call_id": "call-one", "name": "inspect", "arguments": "{}"}},
		} {
			if writeWebSocketEvent(operationContext, connection, event) != nil {
				return
			}
		}
		select {
		case <-allowCompletion:
		case <-operationContext.Done():
			return
		}
		writeWebSocketEvent(operationContext, connection, map[string]any{"type": "response.completed", "response": map[string]any{"id": "tool-response", "output": []any{map[string]any{"type": "reasoning", "id": "reasoning-one", "encrypted_content": "private-reasoning", "summary": []any{map[string]any{"type": "summary_text", "text": "Inspecting"}}}, map[string]any{"type": "function_call", "call_id": "call-one", "name": "inspect", "arguments": "{}"}}, "usage": map[string]any{"input_tokens": 90, "output_tokens": 12, "output_tokens_details": map[string]any{"reasoning_tokens": 9}, "cost": 0.04}}})
	}))
	test.Cleanup(server.Close)
	modelProvider := New(atom.ProviderSpec{Name: "synthetic-plan", Protocol: "responses", Authentication: "chatgpt", APIURL: server.URL, Transport: "websocket", Billing: "subscription"})
	test.Cleanup(func() { modelProvider.Close() })
	modelProvider.SetHeaderResolver(func(context.Context) (http.Header, error) {
		return http.Header{"Authorization": []string{"Bearer account-token"}, "Chatgpt-Account-Id": []string{"account-one"}}, nil
	})
	modelProvider.SetModels([]atom.ModelInfo{{ID: "transport-model@fast", APIModel: "transport-model", ServiceTier: "priority", Reasoning: true, ReasoningEfforts: []string{"high"}, ReasoningSummary: "auto"}})
	stream, operationError := modelProvider.Stream(operationContext, atom.Request{Model: "transport-model@fast", ReasoningEffort: "high", Tools: []atom.ToolSpec{{Name: "inspect", Description: "Inspect data", InputSchema: atom.Schema{JSON: json.RawMessage(`{"type":"object"}`)}}}, Messages: []atom.Message{{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Inspect this"}}}}, Params: map[string]any{"max_output_tokens": 128}})
	testutil.RequireNoError(test, operationError)
	first, operationError := stream.Recv(operationContext)
	testutil.RequireNoError(test, operationError)
	second, operationError := stream.Recv(operationContext)
	testutil.RequireNoError(test, operationError)
	if first.Reasoning != "Inspecting" || second.ToolCall == nil || second.ToolCall.ID != "call-one" {
		test.Fatalf("events were delayed or changed: %#v %#v", first, second)
	}
	close(allowCompletion)
	message, operationError := collectTransportMessage(operationContext, stream)
	testutil.RequireNoError(test, operationError)
	if len(message.ToolCalls) != 0 || message.ProviderState == nil || len(message.ProviderState.Reasoning) != 1 || message.Usage == nil || message.Usage.Output != 12 || message.Usage.Reasoning != 9 || message.Usage.Cost == nil || message.Usage.Cost.Value != 0.04 {
		test.Fatalf("completion lost or duplicated data: %#v", message)
	}
	payload := <-requests
	if payload["model"] != "transport-model" || payload["service_tier"] != "priority" || payload["max_output_tokens"] != nil || payload["reasoning"].(map[string]any)["effort"] != "high" {
		test.Fatalf("preset/plan wire fields changed: %v", payload)
	}
}
