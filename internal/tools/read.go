package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Read struct {
	lineNumbers bool
}

// NewRead fixes the output experiment for the lifetime of the registered tool.
func NewRead(lineNumbers bool) *Read {
	return &Read{lineNumbers: lineNumbers}
}

func (Read) Name() string {
	return "read"
}

func (readTool Read) Description() string {
	description := "Read file text, optionally limited to a 1-based inclusive line range. Relative paths resolve from the instance workspace. Omit both bounds to start at line 1 through EOF. Returned file text is capped at 200 lines or 16 KiB, including display labels, plus a truncation notice. When truncated, use the exact start_line and optional start_byte cursor in the notice to continue without repeating or skipping bytes. Very long lines are paged safely; start_byte is a zero-based byte offset within start_line. Prefer a range when only part is needed."
	if readTool.lineNumbers {
		return description + " Each returned line has an absolute file line-number prefix, N: text. These prefixes are display labels, not file content; exclude them from write and replace inputs."
	}
	return description + " Output is plain text without line-number prefixes."
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
				"description": "Path to an existing text file, relative to the instance workspace or absolute. Symbolic links are resolved before workspace permission checks.",
				"examples": ["src/main.go"]
			},
			"start_line": {
				"type": "integer", "minimum": 1, "default": 1,
				"description": "First file line to return, starting at 1 and including this line. Omitted means line 1. A start beyond EOF is an error; an empty file has zero lines."
			},
			"end_line": {
				"type": "integer", "minimum": 1,
				"description": "Last file line to return, inclusive, and at least start_line. Omitted means through EOF. An end beyond EOF is clamped to EOF. The 200-line/16-KiB output cap still applies and gives a continuation cursor. A trailing newline does not create an extra empty line."
			},
			"start_byte": {
				"type": "integer", "minimum": 0, "default": 0,
				"description": "Zero-based byte offset within start_line only, excluding any displayed line-number prefix. Omitted or zero starts at the line's beginning. Use the exact offset from a truncation notice to continue a long line; it must address a byte in the line without splitting a UTF-8 character. Later lines start at byte zero."
			}
		},
		"required": ["path"]
	}`)
}

func (Read) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (readTool Read) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		Path string `json:"path"`
		fileTextSelection
	}
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return fileReadFailure(call, input.Path, operationError)
	}
	if operationError := operationContext.Err(); operationError != nil {
		return fileReadFailure(call, input.Path, operationError)
	}
	_, _, operationError := input.lineRange.resolveLineBounds()
	if operationError != nil {
		return fileReadFailure(call, input.Path, operationError)
	}
	if input.Path == "" || input.StartByte < 0 {
		return fileReadFailure(call, input.Path, fmt.Errorf("path must not be empty and start_byte must be zero or greater"))
	}
	path, operationError := harness.WorkspacePath(operationContext, input.Path)
	if operationError != nil {
		return fileReadFailure(call, input.Path, operationError)
	}
	file, operationError := os.Open(path)
	if operationError != nil {
		return fileReadFailure(call, input.Path, operationError)
	}
	defer file.Close()
	preview, operationError := readFileTextPreview(operationContext, file, input.fileTextSelection, readTool.lineNumbers)
	if operationError != nil {
		return fileReadFailure(call, input.Path, operationError)
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: preview.render(input.EndLine)}},
	}, nil
}

func fileReadFailure(call atom.ToolCall, path string, cause error) (atom.ToolResult, error) {
	return failedFileOperation(call, path, "read", cause, nil)
}
