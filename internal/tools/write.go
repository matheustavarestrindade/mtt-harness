package tools

import (
	"context"
	"encoding/json"
	"os"

	"github.com/matheustavarestrindade/mtt-harness/atom"
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

func (Write) Check(ctx context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (Write) Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(call.Input, &input); err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "write: the input is not correct"}, err
	}
	if err := os.WriteFile(input.Path, []byte(input.Content), 0o644); err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: err.Error()}, err
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: "the file is written"}},
	}, nil
}
