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

func TestToolsChangedCallbackCanIssueRPCAndReadPages(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			responseWriter.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if request.Method == http.MethodDelete {
			responseWriter.WriteHeader(http.StatusNoContent)
			return
		}
		var message rpcMessage
		if operationError := json.NewDecoder(request.Body).Decode(&message); operationError != nil {
			test.Error(operationError)
			return
		}
		if message.ID == nil {
			responseWriter.WriteHeader(http.StatusAccepted)
			return
		}
		if request.Header.Get("MCP-Protocol-Version") != ProtocolVersion {
			test.Error("missing protocol header")
		}
		var result any
		switch message.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": ProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{"listChanged": true}}}
		case "tools/list":
			var parameters struct {
				Cursor string `json:"cursor"`
			}
			json.Unmarshal(message.Params, &parameters)
			if parameters.Cursor == "next" {
				result = map[string]any{"tools": []map[string]any{{"name": "second"}}}
				break
			}
			result = map[string]any{"tools": []map[string]any{{"name": "first"}}, "nextCursor": "next"}
		case "tools/call":
			responseWriter.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(responseWriter, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/tools/list_changed\"}\n\n")
			data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": map[string]any{"content": []any{}}})
			fmt.Fprintf(responseWriter, "data: %s\n\n", data)
			responseWriter.(http.Flusher).Flush()
			<-request.Context().Done()
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		json.NewEncoder(responseWriter).Encode(map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": result})
	}))
	defer server.Close()
	client, operationError := Start(context.Background(), ServerSpec{Name: "test", URL: server.URL}, 2*time.Second)
	testutil.RequireNoError(test, operationError)
	defer client.Close()
	completed := make(chan error, 1)
	client.OnToolsChanged(func(operationContext context.Context) {
		tools, operationError := client.ListTools(operationContext)
		if operationError == nil && len(tools) != 2 {
			operationError = fmt.Errorf("pagination returned %d tools", len(tools))
		}
		completed <- operationError
	})
	_, operationError = client.CallTool(context.Background(), "first", []byte(`{}`))
	testutil.RequireNoError(test, operationError)
	select {
	case operationError := <-completed:
		testutil.RequireNoError(test, operationError)
	case <-time.After(3 * time.Second):
		test.Fatal("tools-changed callback blocked the RPC reader")
	}
}
