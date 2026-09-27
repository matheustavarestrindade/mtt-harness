package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
)

func TestBridgeLoadsRemoteTools(t *testing.T) {
	h := harness.New()
	reg := registry.New()
	b, err := Start(context.Background(), Config{Harness: h, Registry: reg, Timeout: 30 * time.Second}, "go", "run", "testdata/plugin/main.go")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	tool, ok := reg.Get("echo")
	if !ok {
		t.Fatal("the echo tool is not in the registry")
	}
	if tool.Description() != "Echo the text" {
		t.Fatalf("the description = %q", tool.Description())
	}
	result, err := tool.Run(context.Background(), atom.ToolCall{ID: "call-1", Name: "echo", Input: []byte(`{"text":"hi"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != atom.StatusOK || result.Text() != "echo: hi" {
		t.Fatalf("the result = %+v", result)
	}
	verdict, err := harness.Check(context.Background(), h, atom.StageToolInput, atom.ToolCall{Name: "echo", Input: []byte(`{"text":"hi"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Kind != atom.VerdictAllow {
		t.Fatalf("the verdict = %+v", verdict)
	}
}
