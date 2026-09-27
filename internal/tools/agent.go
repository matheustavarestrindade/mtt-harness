package tools

import (
	"context"
	"encoding/json"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type AgentTaskFunc func(ctx context.Context, task atom.AgentTask) (atom.ToolResult, error)

type Agent struct {
	RunTask AgentTaskFunc
}

func (a *Agent) Name() string {
	return "agent"
}

func (a *Agent) Description() string {
	return "Start a child agent with a task. The child agent gives the result."
}

func (a *Agent) Categories() []string {
	return []string{"agent"}
}

func (a *Agent) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"task":{"type":"string"},"model":{"type":"string"}},"required":["task"]}`)
}

func (a *Agent) Check(ctx context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (a *Agent) Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		Task  string `json:"task"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(call.Input, &input); err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "agent: the input is not correct"}, err
	}
	if a.RunTask == nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "agent: the runner is not attached"}, nil
	}
	return a.RunTask(ctx, atom.AgentTask{Task: input.Task, Model: input.Model})
}
