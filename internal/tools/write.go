package tools

import (
	"bytes"
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
	return "Create or replace an entire file, or replace an inclusive line range in an existing file. Paths resolve from the instance workspace. Range writes keep all other bytes and preserve the block's closing LF/CRLF when non-empty content has no final newline. Parent directories must exist. Edits replace the file atomically and preserve existing permission bits; hard links are not updated." + fileEditFeedbackDescription
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
		Path    string  `json:"path"`
		Content *string `json:"content"`
		lineRange
	}
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return fileEditFailure(call, "write", input.Path, operationError, "Send valid JSON with path and content strings, and optional integer line bounds.")
	}
	if input.Path == "" || input.Content == nil {
		return fileEditFailure(call, "write", input.Path, fmt.Errorf("path and content are required"), "Supply a non-empty path and a content string. Use an empty content string only to create an empty file or delete content.")
	}
	if operationError := operationContext.Err(); operationError != nil {
		return fileEditFailure(call, "write", input.Path, operationError, "Use read before another edit.")
	}
	if _, _, operationError := input.lineRange.resolveLineBounds(); operationError != nil {
		return fileEditFailure(call, "write", input.Path, operationError, "Use read to inspect the current file. Retry with 1-based inclusive line bounds within the file.")
	}
	path, operationError := harness.WorkspacePath(operationContext, input.Path)
	if operationError != nil {
		return fileEditFailure(call, "write", input.Path, operationError, "Verify the file path and instance workspace before retrying.")
	}
	summary, operationError := applyAtomicFileEdit(operationContext, path, !input.hasBounds(), func(original []byte) (fileChange, error) {
		if !input.hasBounds() {
			return fileChange{content: []byte(*input.Content), summary: "Whole-file write."}, nil
		}
		start, end, operationError := input.resolveByteRange(original)
		if operationError != nil {
			return fileChange{}, operationError
		}
		replacement := []byte(*input.Content)
		if len(replacement) > 0 && !bytes.HasSuffix(replacement, []byte("\n")) {
			replacement = append(replacement, closingLineBreak(original[start:end])...)
		}
		updated := make([]byte, 0, start+len(replacement)+len(original)-end)
		updated = append(updated, original[:start]...)
		updated = append(updated, replacement...)
		updated = append(updated, original[end:]...)
		return fileChange{content: updated, summary: fmt.Sprintf("Replaced lines %d-%d of the original file.", countFileLines(original[:start])+1, countFileLines(original[:end]))}, nil
	})
	if operationError != nil {
		return fileEditFailure(call, "write", input.Path, operationError, "Verify the path and use read to inspect the current file. Update the content or line bounds from that fresh read before retrying.")
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: summary}},
	}, nil
}
