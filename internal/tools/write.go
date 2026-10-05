package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Write struct{}

func (Write) Name() string {
	return "write"
}

func (Write) Description() string {
	return "Create or replace an entire file, or replace an inclusive line range in an existing file. Paths resolve from the instance workspace. Range writes keep all other bytes and preserve the block's closing LF/CRLF when non-empty content has no final newline. Parent directories must exist. Edits replace the file atomically and preserve existing permission bits; hard links are not updated. Returns status, resolved path, and before/after line and byte counts. Use return to choose unified diff-only hunks, a diff with surrounding context, an updated-file line range, or the updated file; omitted means a diff with 3 context lines. File/range previews have absolute line-number labels, not file content. All previews stop at 200 lines or 16 KiB of displayed text plus status/truncation notices; file/range previews provide an exact read continuation cursor. Diff comparison regions above 256 KiB or 4000 combined lines, or non-text data, receive an omission notice. Preview limits never limit the edit. Invalid return options or ranges fail before writing; failures explain the cause and recovery."
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
				"description": "Replacement text for the whole file or selected lines, without read/preview line-number prefixes or truncation notices. Empty text deletes the selected lines, or creates/truncates the whole file when no range is supplied. Internal newlines are written as provided."
			},
			"start_line": {
				"type": "integer", "minimum": 1,
				"description": "First line to replace, 1-based and inclusive. Omitted with end_line means line 1. If either bound is supplied, the file and selected lines must already exist; both omitted means whole-file replacement."
			},
			"end_line": {
				"type": "integer", "minimum": 1,
				"description": "Last line to replace, inclusive and at least start_line. Omitted with start_line means through EOF. Unlike read, an end beyond EOF is an error. Ranges apply to the current file at execution time."
			},
			"return": ` + fileReturnInputSchema + `
		},
		"required": ["path", "content"]
	}`)
}

func (Write) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (Write) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		Path    string          `json:"path"`
		Content *string         `json:"content"`
		Return  json.RawMessage `json:"return"`
		lineRange
	}
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return fileEditFailure(call, "write", input.Path, operationError)
	}
	if input.Path == "" || input.Content == nil {
		return fileEditFailure(call, "write", input.Path, fmt.Errorf("path and content are required"))
	}
	returnOptions, operationError := parseFileReturnOptions(input.Return)
	if operationError != nil {
		return fileEditFailure(call, "write", input.Path, operationError)
	}
	if operationError := operationContext.Err(); operationError != nil {
		return fileEditFailure(call, "write", input.Path, operationError)
	}
	if _, _, operationError := input.lineRange.resolveLineBounds(); operationError != nil {
		return fileEditFailure(call, "write", input.Path, operationError)
	}
	path, operationError := harness.WorkspacePath(operationContext, input.Path)
	if operationError != nil {
		return fileEditFailure(call, "write", input.Path, operationError)
	}
	summary, operationError := applyAtomicFileEdit(operationContext, path, !input.hasBounds(), func(original []byte) (fileChange, error) {
		updated, description, operationError := writeFileContent(original, *input.Content, input.lineRange)
		return fileChange{content: updated, summary: description, returnOptions: returnOptions}, operationError
	})
	if operationError != nil {
		return fileEditFailure(call, "write", input.Path, operationError)
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: summary}},
	}, nil
}
