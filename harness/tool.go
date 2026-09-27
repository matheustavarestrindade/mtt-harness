package harness

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Tool interface {
	Name() string
	Description() string
	Categories() []string
	InputSchema() atom.Schema
	Check(ctx context.Context, call atom.ToolCall) atom.Verdict
	Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error)
}
