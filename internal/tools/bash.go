package tools

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Bash struct{}

func (Bash) Name() string {
	return "bash"
}

func (Bash) Description() string {
	return "Start a shell command and monitor the process."
}

func (Bash) Categories() []string {
	return []string{"process", "command"}
}

func (Bash) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"command":{"type":"string"},"notify":{"type":"string","enum":["exit","error","interval"]},"interval":{"type":"integer"}},"required":["command"]}`)
}

func (Bash) Check(ctx context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (Bash) Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	return atom.ToolResult{
		CallID: call.ID,
		Status: atom.StatusError,
		Error:  "tools: bash is not implemented",
	}, errors.New("tools: bash is not implemented")
}
