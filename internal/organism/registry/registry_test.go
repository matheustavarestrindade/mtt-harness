package registry

import (
	"context"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type fakeTool struct {
	name       string
	desc       string
	categories []string
}

func (fakeTool fakeTool) Name() string {
	return fakeTool.name
}
func (fakeTool fakeTool) Description() string {
	return fakeTool.desc
}
func (fakeTool fakeTool) Categories() []string {
	return fakeTool.categories
}
func (fakeTool fakeTool) InputSchema() atom.Schema {
	return atom.Schema{}
}
func (fakeTool fakeTool) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}
func (fakeTool fakeTool) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	return atom.ToolResult{}, nil
}

func names(tools []harness.Tool) []string {
	var list []string
	for _, tool := range tools {
		list = append(list, tool.Name())
	}
	return list
}

func TestFind(test *testing.T) {
	registry := New(harness.New())
	_ = registry.Add(fakeTool{name: "read", desc: "Read a file", categories: []string{"file"}})
	_ = registry.Add(fakeTool{name: "write", desc: "Write a file", categories: []string{"file"}})
	_ = registry.Add(fakeTool{name: "bash", desc: "Start a command", categories: []string{"process"}})
	_ = registry.Add(fakeTool{name: "agent", desc: "Start an agent", categories: []string{"agent"}})

	actual := names(registry.Find("file", "", 0))
	if len(actual) != 2 || actual[0] != "read" || actual[1] != "write" {
		test.Fatalf("find by text = %v", actual)
	}
	actual = names(registry.Find("", "process", 0))
	if len(actual) != 1 || actual[0] != "bash" {
		test.Fatalf("find by category = %v", actual)
	}
	actual = names(registry.Find("COMMAND", "", 0))
	if len(actual) != 1 || actual[0] != "bash" {
		test.Fatalf("find is not case-insensitive: %v", actual)
	}
	actual = names(registry.Find("file", "", 1))
	if len(actual) != 1 {
		test.Fatalf("limit = %v", actual)
	}
	actual = names(registry.Find("nothing", "", 0))
	if len(actual) != 0 {
		test.Fatalf("no match = %v", actual)
	}
	actual = names(registry.Find("file", "", 0))
	again := names(registry.Find("file", "", 0))
	if len(actual) != len(again) || actual[0] != again[0] || actual[1] != again[1] {
		test.Fatalf("find is not deterministic: %v %v", actual, again)
	}
}
