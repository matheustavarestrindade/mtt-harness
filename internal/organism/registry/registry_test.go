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

func (f fakeTool) Name() string             { return f.name }
func (f fakeTool) Description() string      { return f.desc }
func (f fakeTool) Categories() []string     { return f.categories }
func (f fakeTool) InputSchema() atom.Schema { return atom.Schema{} }
func (f fakeTool) Check(ctx context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}
func (f fakeTool) Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	return atom.ToolResult{}, nil
}

func names(tools []harness.Tool) []string {
	var list []string
	for _, tool := range tools {
		list = append(list, tool.Name())
	}
	return list
}

func TestFind(t *testing.T) {
	registry := New()
	_ = registry.Add(fakeTool{name: "read", desc: "Read a file", categories: []string{"file"}})
	_ = registry.Add(fakeTool{name: "write", desc: "Write a file", categories: []string{"file"}})
	_ = registry.Add(fakeTool{name: "bash", desc: "Start a command", categories: []string{"process"}})
	_ = registry.Add(fakeTool{name: "agent", desc: "Start an agent", categories: []string{"agent"}})

	got := names(registry.Find("file", "", 0))
	if len(got) != 2 || got[0] != "read" || got[1] != "write" {
		t.Fatalf("find by text = %v", got)
	}
	got = names(registry.Find("", "process", 0))
	if len(got) != 1 || got[0] != "bash" {
		t.Fatalf("find by category = %v", got)
	}
	got = names(registry.Find("COMMAND", "", 0))
	if len(got) != 1 || got[0] != "bash" {
		t.Fatalf("find is not case-insensitive: %v", got)
	}
	got = names(registry.Find("file", "", 1))
	if len(got) != 1 {
		t.Fatalf("limit = %v", got)
	}
	got = names(registry.Find("nothing", "", 0))
	if len(got) != 0 {
		t.Fatalf("no match = %v", got)
	}
	got = names(registry.Find("file", "", 0))
	again := names(registry.Find("file", "", 0))
	if len(got) != len(again) || got[0] != again[0] || got[1] != again[1] {
		t.Fatalf("find is not deterministic: %v %v", got, again)
	}
}
