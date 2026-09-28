package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestHTTPTransport(test *testing.T) {
	var sessionHeaders []string
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		sessionHeaders = append(sessionHeaders, httpRequest.Header.Get("Mcp-Session-Id"))
		var request rpcMessage
		if operationError := json.NewDecoder(httpRequest.Body).Decode(&request); operationError != nil {
			http.Error(responseWriter, operationError.Error(), http.StatusBadRequest)
			return
		}
		var result any
		switch request.Method {
		case "initialize":
			responseWriter.Header().Set("Mcp-Session-Id", "session-1")
			result = map[string]any{
				"protocolVersion": ProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "remote", "version": "0.1.0"},
			}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{
				"name":        "echo",
				"description": "Echo",
				"inputSchema": map[string]any{"type": "object"},
			}}}
		case "tools/call":
			responseWriter.Header().Set("Content-Type", "text/event-stream")
			answer, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "remote: ok"}},
				},
			})
			fmt.Fprintf(responseWriter, "data: %s\n\n", answer)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(responseWriter).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()

	client, operationError := Start(context.Background(), ServerSpec{Name: "remote", URL: server.URL}, 30*time.Second)
	testutil.RequireNoError(test, operationError)

	defer client.Close()
	operationContext := context.Background()
	tools, operationError := client.ListTools(operationContext)
	testutil.RequireNoError(test, operationError)

	if len(tools) != 1 || tools[0].Name != "echo" {
		test.Fatalf("tools = %+v", tools)
	}
	result, operationError := client.CallTool(operationContext, "echo", []byte(`{"text":"x"}`))
	testutil.RequireNoError(test, operationError)

	if len(result.Content) != 1 || result.Content[0].Text != "remote: ok" {
		test.Fatalf("result = %+v", result)
	}
	for index, header := range sessionHeaders {
		if index > 0 && header != "session-1" {
			test.Fatalf("the session header %d = %q", index, header)
		}
	}
}
