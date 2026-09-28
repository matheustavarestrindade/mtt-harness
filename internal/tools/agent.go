package tools

import (
	"context"
	"encoding/json"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type AgentTaskFunc func(operationContext context.Context, task atom.AgentTask) (atom.ToolResult, error)

type Agent struct {
	RunTask AgentTaskFunc
}

func (agentTool *Agent) Name() string {
	return "agent"
}

func (agentTool *Agent) Description() string {
	return "Start a child agent with a task. The child agent gives the result."
}

func (agentTool *Agent) Categories() []string {
	return []string{"agent"}
}

func (agentTool *Agent) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"task":{"type":"string"},"model":{"type":"string"}},"required":["task"]}`)
}

func (agentTool *Agent) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (agentTool *Agent) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		Task  string `json:"task"`
		Model string `json:"model"`
	}
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "agent: the input is not correct"}, operationError
	}
	if agentTool.RunTask == nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "agent: the runner is not attached"}, nil
	}
	return agentTool.RunTask(operationContext, atom.AgentTask{Task: input.Task, Model: input.Model})
}
