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
	return "Run a delegated task in a separate child session in the same instance and workspace. This call waits for the child to call finish, then returns its result and usage summary. Child agents have separate conversation history, not a copy of the parent history. Agent depth limits and the instance model allowlist apply."
}

func (agentTool *Agent) Categories() []string {
	return []string{"agent"}
}

func (agentTool *Agent) InputSchema() atom.Schema {
	return schema(`{
		"type": "object",
		"properties": {
			"task": {
				"type": "string",
				"description": "Self-contained instructions for the child, including needed context, relevant paths, and the expected deliverable. The child receives this text as its initial user message and must return its final result through finish."
			},
			"model": {
				"type": "string",
				"description": "Model ID from the instance's available models, preferably provider/model. A bare ID is accepted only when unambiguous. Omit to select the instance default model, not necessarily the parent's model."
			}
		},
		"required": ["task"]
	}`)
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
