package tools

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Agent struct{}

func (Agent) Name() string {
	return "agent"
}

func (Agent) Description() string {
	return "Start a child agent with a task."
}

func (Agent) Categories() []string {
	return []string{"agent"}
}

func (Agent) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"task":{"type":"string"},"model":{"type":"string"}},"required":["task"]}`)
}

func (Agent) Check(ctx context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (Agent) Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	return atom.ToolResult{
		CallID: call.ID,
		Status: atom.StatusError,
		Error:  "tools: agent is not implemented",
	}, errors.New("tools: agent is not implemented")
}
