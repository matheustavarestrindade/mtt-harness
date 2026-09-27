package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPTransport(t *testing.T) {
	var sessionHeaders []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionHeaders = append(sessionHeaders, r.Header.Get("Mcp-Session-Id"))
		var request rpcMessage
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var result any
		switch request.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-1")
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
			w.Header().Set("Content-Type", "text/event-stream")
			answer, _ := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"id":      request.ID,
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "remote: ok"}},
				},
			})
			fmt.Fprintf(w, "data: %s\n\n", answer)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()

	client, err := Start(context.Background(), ServerSpec{Name: "remote", URL: server.URL}, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()
	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %+v", tools)
	}
	result, err := client.CallTool(ctx, "echo", []byte(`{"text":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "remote: ok" {
		t.Fatalf("result = %+v", result)
	}
	for index, header := range sessionHeaders {
		if index > 0 && header != "session-1" {
			t.Fatalf("the session header %d = %q", index, header)
		}
	}
}
