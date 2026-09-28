package tools

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Finish struct{}

func (Finish) Name() string {
	return "finish"
}

func (Finish) Description() string {
	return "Stop the agent and give the result."
}

func (Finish) Categories() []string {
	return []string{"agent"}
}

func (Finish) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"result":{"type":"string"}},"required":["result"]}`)
}

func (Finish) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (Finish) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: "the agent stops"}},
	}, nil
}
