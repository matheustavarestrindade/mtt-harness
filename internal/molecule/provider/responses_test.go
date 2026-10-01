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
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestResponsesStreamsToolBeforeCompletionAndKeepsContinuation(test *testing.T) {
	operationContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	finish := make(chan struct{})
	requests := make(chan map[string]any, 1)
	reasoning := `{"type":"reasoning","id":"reason-1","encrypted_content":"opaque","summary":[]}`
	call := `{"type":"function_call","id":"item-1","call_id":"call-1","name":"read","arguments":"{\"path\":\"file.txt\"}"}`
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/responses" || request.Header.Get("Authorization") != "Bearer example-key" {
			http.Error(responseWriter, "unexpected transport", 400)
			return
		}
		var payload map[string]any
		if json.NewDecoder(request.Body).Decode(&payload) != nil {
			http.Error(responseWriter, "invalid JSON", 400)
			return
		}
		requests <- payload
		responseWriter.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(responseWriter, "event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"output_index\":1,\"item\":%s}\n\n", call)
		responseWriter.(http.Flusher).Flush()
		select {
		case <-finish:
		case <-request.Context().Done():
			return
		}
		fmt.Fprintf(responseWriter, "data: {\"type\":\"response.completed\",\"response\":{\"output\":[%s,%s],\"usage\":{\"input_tokens\":100,\"input_tokens_details\":{\"cached_tokens\":40},\"output_tokens\":20,\"output_tokens_details\":{\"reasoning_tokens\":12}}}}\n\n", reasoning, call)
	}))
	defer server.Close()
	standard := New(atom.ProviderSpec{Name: "openai", Protocol: "responses", APIURL: server.URL, Authentication: "api_key"})
	standard.SetKeyResolver(func(context.Context, string, string) (string, error) { return "example-key", nil })
	stream, operationError := standard.Stream(operationContext, atom.Request{
		Model: "example-responses-model",
		Messages: []atom.Message{
			{Role: atom.RoleSystem, Content: []atom.Content{{Type: atom.Text, Text: "system instructions"}}},
			{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "read file"}}},
		},
		Tools: []atom.ToolSpec{{Name: "read", Description: "Read a file", InputSchema: atom.Schema{JSON: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)}}},
	})
	testutil.RequireNoError(test, operationError)
	part, operationError := stream.Recv(operationContext)
	testutil.RequireNoError(test, operationError)
	if part.ToolCall == nil || part.ToolCall.ID != "call-1" || part.ToolIndex == nil || *part.ToolIndex != 1 {
		test.Fatalf("early call = %#v", part)
	}
	close(finish)
	message := atom.Message{Role: atom.RoleAssistant, ToolCalls: []atom.ToolCall{*part.ToolCall}}
	var usage *atom.Usage
	for {
		part, operationError = stream.Recv(operationContext)
		if errors.Is(operationError, io.EOF) {
			break
		}
		testutil.RequireNoError(test, operationError)
		if part.ToolCall != nil {
			test.Fatal("duplicate function call at completion")
		}
		if part.ProviderState != nil {
			message.ProviderState = part.ProviderState
		}
		if part.Usage != nil {
			usage = part.Usage
		}
	}
	if usage == nil || usage.Input != 60 || usage.CacheRead != 40 || usage.Output != 20 || usage.Reasoning != 12 {
		test.Fatalf("usage = %#v", usage)
	}
	if message.ProviderState == nil || len(message.ProviderState.Reasoning) != 1 {
		test.Fatalf("reasoning state = %#v", message.ProviderState)
	}
	payload := <-requests
	if payload["instructions"] != "system instructions" || payload["store"] != false || payload["stream"] != true {
		test.Fatalf("request = %#v", payload)
	}
	tools := payload["tools"].([]any)
	tool := tools[0].(map[string]any)
	if tool["name"] != "read" || tool["type"] != "function" || tool["strict"] != false || tool["parameters"] == nil || tool["function"] != nil {
		test.Fatalf("Responses tool schema = %#v", tool)
	}
	input, _, operationError := responseInput([]atom.Message{message, {Role: atom.RoleTool, ToolCallID: "call-1", Content: []atom.Content{{Type: atom.Text, Text: "contents"}}}}, "openai")
	testutil.RequireNoError(test, operationError)
	if len(input) != 3 || input[0].(map[string]any)["type"] != "reasoning" || input[2].(map[string]any)["call_id"] != "call-1" {
		test.Fatalf("continuation = %#v", input)
	}
	other, _, operationError := responseInput([]atom.Message{message}, "deepseek")
	testutil.RequireNoError(test, operationError)
	if len(other) != 1 || other[0].(map[string]any)["type"] != "function_call" {
		test.Fatalf("cross-provider state leaked: %#v", other)
	}
	encoded, operationError := json.Marshal(message)
	testutil.RequireNoError(test, operationError)
	if strings.Contains(string(encoded), "opaque") {
		test.Fatal("opaque reasoning leaked through public message JSON")
	}
}

func TestResponsesRejectsTruncatedFailedAndMalformedStreams(test *testing.T) {
	for _, body := range []string{
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n",
		"data: {\"type\":\"response.failed\"}\n\n",
		"data: {\"type\":\"response.incomplete\"}\n\n",
		"data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"call_id\":\"c\",\"name\":\"read\",\"arguments\":\"{\"}}\n\n",
	} {
		stream := &httpStream{parts: make(chan atom.ResponsePart, 64)}
		parseResponses(context.Background(), io.NopCloser(strings.NewReader(body)), stream, "openai")
		for range stream.parts {
		}
		if stream.operationError == nil {
			test.Fatalf("accepted invalid stream %q", body)
		}
	}
}

func TestResponsesMultilineEventsAndDeepSeekReasoning(test *testing.T) {
	body := "event: response.completed\ndata: {\"type\":\"response.completed\",\ndata: \"response\":{\"output\":[{\"type\":\"reasoning\",\"content\":[{\"type\":\"reasoning_text\",\"text\":\"retained reasoning\"}]},{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"answer\"}]}]}}\n\n"
	stream := &httpStream{parts: make(chan atom.ResponsePart, 64)}
	parseResponses(context.Background(), io.NopCloser(strings.NewReader(body)), stream, "deepseek")
	var state *atom.ProviderState
	text := ""
	for part := range stream.parts {
		text += part.Text
		if part.ProviderState != nil {
			state = part.ProviderState
		}
	}
	testutil.RequireNoError(test, stream.operationError)
	if text != "answer" || state == nil || !strings.Contains(string(state.Reasoning[0]), "retained reasoning") {
		test.Fatalf("text/state = %q %#v", text, state)
	}
}
