package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestMCPConnectionsLiveUntilApplicationCleanup(test *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			responseWriter.WriteHeader(http.StatusNoContent)
			return
		}
		var message struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		if operationError := json.NewDecoder(request.Body).Decode(&message); operationError != nil {
			test.Error(operationError)
			return
		}
		if message.ID == nil {
			responseWriter.WriteHeader(http.StatusAccepted)
			return
		}
		var result any = map[string]any{}
		switch message.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}}}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "echo", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			result = map[string]any{"content": []map[string]string{{"type": "text", "text": "alive"}}}
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		json.NewEncoder(responseWriter).Encode(map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": result})
	}))
	defer server.Close()
	configurationPath := filepath.Join(test.TempDir(), "mcp.json")
	configuration, operationError := json.Marshal(map[string]any{"servers": []map[string]any{{"name": "test", "url": server.URL, "enabled": true}}})
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, os.WriteFile(configurationPath, configuration, 0600))
	toolRegistry := registry.New(harness.New(), nil)
	cleanup, operationError := loadMCP(context.Background(), configurationPath, toolRegistry)
	testutil.RequireNoError(test, operationError)
	defer cleanup()
	tool, found := toolRegistry.Get("mcp__test__echo")
	if !found {
		test.Fatal("MCP tool was not registered")
	}
	result, operationError := tool.Run(context.Background(), atom.ToolCall{Input: []byte(`{}`)})
	testutil.RequireNoError(test, operationError)
	if result.Status != atom.StatusOK || result.Text() != "alive" {
		test.Fatalf("loader closed the connection: %+v", result)
	}
	testutil.RequireNoError(test, cleanup())
	if _, found := toolRegistry.Get("mcp__test__echo"); found {
		test.Fatal("shutdown left a stale MCP registration")
	}
}
