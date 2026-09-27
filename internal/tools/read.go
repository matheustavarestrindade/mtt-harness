package tools

import (
	"context"
	"encoding/json"
	"os"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Read struct{}

func (Read) Name() string {
	return "read"
}

func (Read) Description() string {
	return "Read a file from the workspace."
}

func (Read) Categories() []string {
	return []string{"file"}
}

func (Read) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)
}

func (Read) Check(ctx context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (Read) Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(call.Input, &input); err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "read: the input is not correct"}, err
	}
	data, err := os.ReadFile(input.Path)
	if err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: err.Error()}, err
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: string(data)}},
	}, nil
}
