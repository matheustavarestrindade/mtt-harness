package tools

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Write struct{}

func (Write) Name() string {
	return "write"
}

func (Write) Description() string {
	return "Create or replace an entire file, or replace an inclusive line range in an existing file. Paths resolve from the instance workspace. Range writes keep all other bytes and preserve the block's closing LF/CRLF when non-empty content has no final newline. Parent directories must exist. Edits replace the file atomically and preserve existing permission bits; hard links are not updated."
}

func (Write) Categories() []string {
	return []string{"file"}
}

func (Write) InputSchema() atom.Schema {
	return schema(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Destination file path, relative to the instance workspace or absolute. The parent directory must exist. Symbolic links are resolved before workspace permission checks.",
				"examples": ["src/main.go"]
			},
			"content": {
				"type": "string",
				"description": "Replacement text for the whole file or selected lines, without read's line-number prefixes. Empty text deletes the selected lines, or creates/truncates the whole file when no range is supplied. Internal newlines are written as provided."
			},
			"start_line": {
				"type": "integer", "minimum": 1,
				"description": "First line to replace, 1-based and inclusive. Omitted with end_line means line 1. If either bound is supplied, the file and selected lines must already exist; both omitted means whole-file replacement."
			},
			"end_line": {
				"type": "integer", "minimum": 1,
				"description": "Last line to replace, inclusive and at least start_line. Omitted with start_line means through EOF. Unlike read, an end beyond EOF is an error. Ranges apply to the current file at execution time."
			}
		},
		"required": ["path", "content"]
	}`)
}

func (Write) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (Write) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		lineRange
	}
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "write: the input is not correct"}, operationError
	}
	if operationError := operationContext.Err(); operationError != nil {
		return atom.ToolResult{}, operationError
	}
	if _, _, operationError := input.lineRange.bounds(); operationError != nil {
		return atom.ToolResult{}, operationError
	}
	path, operationError := harness.WorkspacePath(operationContext, input.Path)
	if operationError != nil {
		return atom.ToolResult{}, operationError
	}
	summary, operationError := updateFile(operationContext, path, !input.specified(), func(original []byte) (fileChange, error) {
		if !input.specified() {
			return fileChange{content: []byte(input.Content), summary: "the file is written"}, nil
		}
		start, end, operationError := input.byteRange(original)
		if operationError != nil {
			return fileChange{}, operationError
		}
		replacement := []byte(input.Content)
		if len(replacement) > 0 && !bytes.HasSuffix(replacement, []byte("\n")) {
			replacement = append(replacement, closingLineBreak(original[start:end])...)
		}
		updated := make([]byte, 0, start+len(replacement)+len(original)-end)
		updated = append(updated, original[:start]...)
		updated = append(updated, replacement...)
		updated = append(updated, original[end:]...)
		return fileChange{content: updated, summary: "the selected lines are written"}, nil
	})
	if operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: summary}},
	}, nil
}
