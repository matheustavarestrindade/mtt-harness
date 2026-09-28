package tools

import (
	"context"
	"encoding/json"
	"os"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Write struct{}

func (Write) Name() string {
	return "write"
}

func (Write) Description() string {
	return "Write a file in the workspace."
}

func (Write) Categories() []string {
	return []string{"file"}
}

func (Write) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`)
}

func (Write) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (Write) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "write: the input is not correct"}, operationError
	}
	if operationError := operationContext.Err(); operationError != nil {
		return atom.ToolResult{}, operationError
	}
	path, operationError := harness.WorkspacePath(operationContext, input.Path)
	if operationError != nil {
		return atom.ToolResult{}, operationError
	}
	if operationError := os.WriteFile(path, []byte(input.Content), 0o644); operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: "the file is written"}},
	}, nil
}
