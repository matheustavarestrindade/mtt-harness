package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

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
	description := "Read file text, optionally limited to a 1-based, inclusive line range. Relative paths resolve from the instance workspace. Omit both bounds to read the whole file; prefer a range when only part is needed."
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
				"description": "Last file line to return, inclusive, and at least start_line. Omitted means through EOF. An end beyond EOF is clamped to EOF. A trailing newline does not create an extra empty line."
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
		lineRange
	}
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "read: the input is not correct"}, operationError
	}
	if operationError := operationContext.Err(); operationError != nil {
		return atom.ToolResult{}, operationError
	}
	start, end, operationError := input.lineRange.resolveLineBounds()
	if operationError != nil {
		return atom.ToolResult{}, operationError
	}
	path, operationError := harness.WorkspacePath(operationContext, input.Path)
	if operationError != nil {
		return atom.ToolResult{}, operationError
	}
	file, operationError := os.Open(path)
	if operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	var output strings.Builder
	lineNumber := 0
	for {
		if operationError := operationContext.Err(); operationError != nil {
			return atom.ToolResult{}, operationError
		}
		line, readError := reader.ReadString('\n')
		if readError != nil && readError != io.EOF {
			return atom.ToolResult{}, readError
		}
		if len(line) > 0 {
			lineNumber++
			if lineNumber >= start {
				if readTool.lineNumbers {
					fmt.Fprintf(&output, "%d: ", lineNumber)
				}
				output.WriteString(line)
			}
		}
		if readError == io.EOF || (end > 0 && lineNumber == end) {
			break
		}
	}
	if input.hasBounds() && start > lineNumber {
		return atom.ToolResult{}, fmt.Errorf("start_line %d exceeds the file's %d lines", start, lineNumber)
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: output.String()}},
	}, nil
}
