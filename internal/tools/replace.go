package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Replace struct{}

func (Replace) Name() string {
	return "replace"
}
func (Replace) Categories() []string {
	return []string{"file"}
}
func (Replace) Description() string {
	return "Replace literal, case-sensitive text in an existing file, optionally within a 1-based inclusive line range. Choose first (default), last, or all non-overlapping matches. Matching is not regex-based; matches may span lines but cannot cross the selected range. No match is an error and leaves the file unchanged. Successful edits atomically replace the file and preserve permission bits; hard links are not updated."
}

func (Replace) InputSchema() atom.Schema {
	return schema(`{
		"type": "object",
		"properties": {
			"path": {"type":"string", "description":"Existing text file path, relative to the instance workspace or absolute. Symbolic links are resolved before workspace permission checks."},
			"old_text": {"type":"string", "minLength":1, "description":"Non-empty literal text to find. Case, whitespace, and LF/CRLF bytes must match exactly. Do not include read's display line numbers. Multiline matches must fit entirely within the selected range."},
			"new_text": {"type":"string", "description":"Literal replacement text, written exactly as supplied. An empty string deletes matched text. Dollar signs and backslashes are not regex replacement expressions; inserted text is not searched again."},
			"mode": {"type":"string", "enum":["first","last","all"], "default":"first", "description":"Which matches to replace within the selected range: first replaces the earliest match; last replaces the latest match; all replaces non-overlapping matches from left to right. Omitted defaults to first."},
			"start_line": {"type":"integer", "minimum":1, "default":1, "description":"First line of the search region, 1-based and inclusive. Omitted means line 1. Explicit bounds must refer to existing lines; both omitted searches the whole file."},
			"end_line": {"type":"integer", "minimum":1, "description":"Last line of the search region, inclusive and at least start_line. Omitted means EOF; an explicit end beyond EOF is an error. Line numbers refer to the file before this edit. A trailing newline does not create an extra empty line."}
		},
		"required": ["path","old_text","new_text"]
	}`)
}

func (Replace) Check(context.Context, atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (Replace) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		Path    string  `json:"path"`
		OldText string  `json:"old_text"`
		NewText *string `json:"new_text"`
		Mode    *string `json:"mode"`
		lineRange
	}
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return atom.ToolResult{}, operationError
	}
	if input.OldText == "" {
		return atom.ToolResult{}, fmt.Errorf("replace: old_text must not be empty")
	}
	if input.NewText == nil {
		return atom.ToolResult{}, fmt.Errorf("replace: new_text is required; use an empty string to delete matches")
	}
	mode := "first"
	if input.Mode != nil {
		mode = *input.Mode
	}
	if mode != "first" && mode != "last" && mode != "all" {
		return atom.ToolResult{}, fmt.Errorf("replace: mode must be first, last, or all")
	}
	if _, _, operationError := input.bounds(); operationError != nil {
		return atom.ToolResult{}, operationError
	}
	path, operationError := harness.WorkspacePath(operationContext, input.Path)
	if operationError != nil {
		return atom.ToolResult{}, operationError
	}
	summary, operationError := updateFile(operationContext, path, false, func(original []byte) (fileChange, error) {
		start, end, operationError := input.byteRange(original)
		if operationError != nil {
			return fileChange{}, operationError
		}
		selected := original[start:end]
		oldText, newText := []byte(input.OldText), []byte(*input.NewText)
		count := bytes.Count(selected, oldText)
		if count == 0 {
			return fileChange{}, fmt.Errorf("replace: old_text was not found in the selected range")
		}
		var replacement []byte
		if mode == "last" {
			index := bytes.LastIndex(selected, oldText)
			replacement = append(replacement, selected[:index]...)
			replacement = append(replacement, newText...)
			replacement = append(replacement, selected[index+len(oldText):]...)
			count = 1
		} else {
			limit := -1
			if mode == "first" {
				limit = 1
				count = 1
			}
			replacement = bytes.Replace(selected, oldText, newText, limit)
		}
		updated := make([]byte, 0, start+len(replacement)+len(original)-end)
		updated = append(updated, original[:start]...)
		updated = append(updated, replacement...)
		updated = append(updated, original[end:]...)
		return fileChange{content: updated, summary: fmt.Sprintf("replaced %d match(es)", count)}, nil
	})
	if operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
	}
	return atom.ToolResult{CallID: call.ID, Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: summary}}}, nil
}
