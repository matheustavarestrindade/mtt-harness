package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestClientListsAndCallsTools(test *testing.T) {
	client, operationError := Start(context.Background(), ServerSpec{
		Name:    "test",
		Command: "go",
		Args:    []string{"run", "testdata/server/main.go"},
	}, 60*time.Second)
	testutil.RequireNoError(test, operationError)

	defer client.Close()

	operationContext := context.Background()
	tools, operationError := client.ListTools(operationContext)
	testutil.RequireNoError(test, operationError)

	if len(tools) != 1 || tools[0].Name != "echo" {
		test.Fatalf("tools = %+v", tools)
	}
	if !strings.Contains(string(tools[0].InputSchema), "text") {
		test.Fatalf("input schema = %s", tools[0].InputSchema)
	}

	tool := &Tool{Client: client, Server: "test", Spec: tools[0]}
	if tool.Name() != "mcp__test__echo" {
		test.Fatalf("tool name = %q", tool.Name())
	}
	if len(tool.Categories()) != 2 || tool.Categories()[0] != "mcp" || tool.Categories()[1] != "test" {
		test.Fatalf("categories = %+v", tool.Categories())
	}
	result, operationError := tool.Run(operationContext, atom.ToolCall{ID: "call-1", Name: tool.Name(), Input: []byte(`{"text":"hi"}`)})
	testutil.RequireNoError(test, operationError)

	if result.Status != atom.StatusOK || result.Text() != "echo: hi" {
		test.Fatalf("result = %+v", result)
	}

	call, operationError := client.CallTool(operationContext, "missing", []byte(`{}`))
	testutil.RequireNoError(test, operationError)

	if !call.IsError {
		test.Fatalf("the missing tool does not give an error: %+v", call)
	}
}
