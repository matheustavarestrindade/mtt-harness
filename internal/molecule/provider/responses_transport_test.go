package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestResponsesTransportConfigurationAndPrivateHeaders(test *testing.T) {
	path := filepath.Join(test.TempDir(), "providers.json")
	testutil.RequireNoError(test, os.WriteFile(path, []byte(`{"providers":[{"name":"configured","protocol":"responses","api_url":"https://api.example/v1","transport":"auto","websocket_url":"wss://socket.example/responses","websocket_headers":{"OpenAI-Beta":"fixture-beta"}}]}`), 0600))
	configurations, operationError := LoadFile(path)
	testutil.RequireNoError(test, operationError)
	configuration := configurations[0]
	if configuration.Spec.Transport != "auto" || configuration.Spec.WebSocketURL != "wss://socket.example/responses" || configuration.WebSocketHeaders.Get("OpenAI-Beta") != "fixture-beta" {
		test.Fatalf("transport configuration was lost: %#v", configuration.Spec)
	}
	public, _ := json.Marshal(configuration.Spec)
	if strings.Contains(string(public), "fixture-beta") {
		test.Fatal("private handshake headers leaked through public provider metadata")
	}
	for _, entry := range []fileEntry{
		{Name: "invalid", Protocol: "chat_completions", APIURL: "https://api.example", Transport: "websocket"},
		{Name: "invalid", Protocol: "responses", APIURL: "https://api.example", Transport: "unknown"},
		{Name: "invalid", Protocol: "responses", APIURL: "https://api.example", WebSocketURL: "https://api.example/socket"},
		{Name: "invalid", Protocol: "responses", APIURL: "https://api.example", WebSocketURL: "wss://secret:token@api.example/socket"},
		{Name: "invalid", Protocol: "responses", APIURL: "https://api.example", WebSocketHeaders: map[string]string{"Authorization": "Bearer secret"}},
		{Name: "invalid", Protocol: "responses", APIURL: "https://api.example", WebSocketHeaders: map[string]string{"Sec-WebSocket-Key": "key"}},
		{Name: "invalid", Protocol: "responses", APIURL: "https://api.example", WebSocketHeaders: map[string]string{"X-Header": "bad\r\nvalue"}},
	} {
		if operationError := validateProviderEntry(entry); operationError == nil {
			test.Fatalf("invalid transport configuration accepted: %#v", entry)
		}
	}
	for _, mode := range []string{"", "http", "auto", "websocket"} {
		testutil.RequireNoError(test, validateProviderEntry(fileEntry{Name: "valid", Protocol: "responses", APIURL: "https://api.example", Transport: mode}))
	}
}

func TestResponsesWebSocketUsesConfiguredEndpointAndPreservesMedia(test *testing.T) {
	operationContext := transportTestContext(test, "workspace", "session")
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/custom-socket" {
			http.NotFound(responseWriter, request)
			return
		}
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
		completeWebSocketResponse(operationContext, connection, "media", "Seen")
	}))
	test.Cleanup(server.Close)
	modelProvider := New(atom.ProviderSpec{Name: "configured", Protocol: "responses", APIURL: server.URL + "/different-http", WebSocketURL: strings.Replace(server.URL, "http://", "ws://", 1) + "/custom-socket", Transport: "websocket"})
	test.Cleanup(func() { modelProvider.Close() })
	stream, operationError := modelProvider.Stream(operationContext, atom.Request{Model: "media-model", Messages: []atom.Message{{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Review"}, {Type: atom.Image, MIME: "image/png", Data: []byte("image bytes")}, {Type: atom.File, MIME: "application/pdf", Filename: "example.pdf", Data: []byte("pdf bytes")}, {Type: atom.Video, MIME: "video/mp4", Data: []byte("video bytes")}}}}})
	testutil.RequireNoError(test, operationError)
	_, operationError = collectTransportMessage(operationContext, stream)
	testutil.RequireNoError(test, operationError)
	content := (<-requests)["input"].([]any)[0].(map[string]any)["content"].([]any)
	for index, expected := range []struct{ key, value string }{{"text", "Review"}, {"image_url", "data:image/png;base64,aW1hZ2UgYnl0ZXM="}, {"file_data", "data:application/pdf;base64,cGRmIGJ5dGVz"}, {"video_url", "data:video/mp4;base64,dmlkZW8gYnl0ZXM="}} {
		if content[index].(map[string]any)[expected.key] != expected.value {
			test.Fatalf("media changed on WebSocket transport: %v", content[index])
		}
	}
}

func TestResponsesHTTP503ReportsObservedFactsAndRedactsCredentials(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.Header().Set("X-Request-ID", "upstream-identifier")
		responseWriter.Header().Set("Retry-After", "3")
		responseWriter.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(responseWriter, `{"error":{"type":"server_error","code":"overloaded","message":"Temporary overload; echoed private-key"}}`)
	}))
	test.Cleanup(server.Close)
	modelProvider := New(atom.ProviderSpec{Name: "compatible", Protocol: "responses", APIURL: server.URL, Authentication: "api_key"})
	defer modelProvider.Close()
	modelProvider.SetKeyResolver(func(context.Context, string, string) (string, error) { return "private-key", nil })
	_, operationError := modelProvider.Stream(context.Background(), atom.Request{Model: "transport-model"})
	if operationError == nil {
		test.Fatal("HTTP 503 succeeded")
	}
	for _, expected := range []string{"HTTP 503", "request_bytes=", "upstream-identifier", "retry_after=3", "code=overloaded", "Temporary overload", "[redacted]"} {
		if !strings.Contains(operationError.Error(), expected) {
			test.Fatalf("missing observed error metadata %q: %v", expected, operationError)
		}
	}
	for _, unexpected := range []string{"private-key", "check provider access", "payload too large"} {
		if strings.Contains(operationError.Error(), unexpected) {
			test.Fatalf("error leaked credentials or invented a cause: %v", operationError)
		}
	}
}

func TestResponsesContinuationNormalizesStoredArgumentJSONWithoutChangingInput(test *testing.T) {
	original := map[string]any{"type": "function_call", "call_id": "call", "name": "inspect", "arguments": `{"z":9007199254740993,"a":"literal"}`}
	restored := map[string]any{"type": "function_call", "call_id": "call", "name": "inspect", "arguments": `{"a": "literal", "z": 9007199254740993}`}
	first, operationError := responsesItemFingerprints([]any{original})
	testutil.RequireNoError(test, operationError)
	second, operationError := responsesItemFingerprints([]any{restored})
	testutil.RequireNoError(test, operationError)
	if first[0] != second[0] {
		test.Fatal("JSONB whitespace/key order prevented continuation")
	}
	restored["arguments"] = `{"a":"literal","z":9007199254740992}`
	changed, operationError := responsesItemFingerprints([]any{restored})
	testutil.RequireNoError(test, operationError)
	if changed[0] == first[0] {
		test.Fatal("tool-argument comparison rounded a material integer change")
	}
	if original["arguments"] != `{"z":9007199254740993,"a":"literal"}` {
		test.Fatal("fingerprinting rewrote the request")
	}
}
