package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func TestClientListsAndCallsTools(t *testing.T) {
	client, err := Start(context.Background(), ServerSpec{
		Name:    "test",
		Command: "go",
		Args:    []string{"run", "testdata/server/main.go"},
	}, 60*time.Second)
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
	if !strings.Contains(string(tools[0].InputSchema), "text") {
		t.Fatalf("input schema = %s", tools[0].InputSchema)
	}

	tool := &Tool{Client: client, Server: "test", Spec: tools[0]}
	if tool.Name() != "mcp__test__echo" {
		t.Fatalf("tool name = %q", tool.Name())
	}
	if len(tool.Categories()) != 2 || tool.Categories()[0] != "mcp" || tool.Categories()[1] != "test" {
		t.Fatalf("categories = %+v", tool.Categories())
	}
	result, err := tool.Run(ctx, atom.ToolCall{ID: "call-1", Name: tool.Name(), Input: []byte(`{"text":"hi"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != atom.StatusOK || result.Text() != "echo: hi" {
		t.Fatalf("result = %+v", result)
	}

	call, err := client.CallTool(ctx, "missing", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !call.IsError {
		t.Fatalf("the missing tool does not give an error: %+v", call)
	}
}
