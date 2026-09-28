package tools

import (
	"context"
	"encoding/json"
	"os"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Read struct{}

func (Read) Name() string {
	return "read"
}

func (Read) Description() string {
	return "Read an entire file and return its contents as text. Relative paths resolve from the instance workspace. This tool has no line-range or binary-media decoding options."
}

func (Read) Categories() []string {
	return []string{"file"}
}

func (Read) InputSchema() atom.Schema {
	return schema(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Path to an existing file, relative to the instance workspace or absolute. Symbolic links are resolved before workspace permission checks. The result contains the entire file as text.",
				"examples": ["src/main.go"]
			}
		},
		"required": ["path"]
	}`)
}

func (Read) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (Read) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		Path string `json:"path"`
	}
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "read: the input is not correct"}, operationError
	}
	if operationError := operationContext.Err(); operationError != nil {
		return atom.ToolResult{}, operationError
	}
	path, operationError := harness.WorkspacePath(operationContext, input.Path)
	if operationError != nil {
		return atom.ToolResult{}, operationError
	}
	data, operationError := os.ReadFile(path)
	if operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: string(data)}},
	}, nil
}
