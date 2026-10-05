package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

const fileReturnInputSchema = `{
	"type": "object",
	"description": "Select feedback from this write's updated-file snapshot, without a second read. Omitted means surrounding with size 3. Status, path, and line/byte totals always remain. Limits affect returned text only, never the edit. Invalid return options or a start beyond the updated file's EOF fail before writing.",
	"default": {"type":"surrounding","size":3},
	"properties": {
		"type": {"type":"string", "enum":["diff","surrounding","lines","file"], "description":"diff returns only change hunks with zero unchanged context; surrounding returns the diff plus size unchanged lines above/below each hunk; lines returns selected lines from the updated file; file returns the updated file from line 1. lines/file do not include a diff. All previews stop at 200 lines or 16 KiB; text previews provide a read continuation cursor."},
		"size": {"type":"integer", "minimum":0, "maximum":100, "default":3, "description":"Only for type surrounding: unchanged context lines above and below each diff hunk. Omitted means 3; zero is equivalent to diff. The output cap can reduce the visible context; maximum accepted size is 100."},
		"start_line": {"type":"integer", "minimum":1, "default":1, "description":"Only for type lines: first line to display AFTER the write, 1-based and inclusive. Omitted means line 1. This is independent of the top-level start_line that selects old lines to replace. A start beyond updated EOF is an error and prevents the write."},
		"end_line": {"type":"integer", "minimum":1, "description":"Only for type lines: last updated-file line to display, inclusive and at least return.start_line. Omitted means EOF; oversized ends clamp to EOF. The 200-line/16-KiB preview cap still applies."}
	},
	"required": ["type"],
	"additionalProperties": false
}`

type fileReturnOptions struct {
	Type string `json:"type"`
	Size *int   `json:"size,omitempty"`
	lineRange
}

func parseFileReturnOptions(input json.RawMessage) (fileReturnOptions, error) {
	if len(input) == 0 {
		return fileReturnOptions{}, nil
	}
	var options *fileReturnOptions
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if operationError := decoder.Decode(&options); operationError != nil {
		return fileReturnOptions{}, fmt.Errorf("return must be an object with supported output settings: %w", operationError)
	}
	if options == nil {
		return fileReturnOptions{}, fmt.Errorf("return must be an object, not null")
	}
	switch options.Type {
	case "diff", "surrounding", "lines", "file":
	default:
		return fileReturnOptions{}, fmt.Errorf("return.type must be diff, surrounding, lines, or file")
	}
	if options.Size != nil {
		if options.Type != "surrounding" {
			return fileReturnOptions{}, fmt.Errorf("return.size is only valid for type surrounding")
		}
		if *options.Size < 0 || *options.Size > 100 {
			return fileReturnOptions{}, fmt.Errorf("return.size must be between 0 and 100 lines")
		}
	}
	if options.hasBounds() && options.Type != "lines" {
		return fileReturnOptions{}, fmt.Errorf("return.start_line and return.end_line are only valid for type lines")
	}
	if _, _, operationError := options.resolveLineBounds(); operationError != nil {
		return fileReturnOptions{}, fmt.Errorf("return.%w", operationError)
	}
	return *options, nil
}

func (options fileReturnOptions) diffContextLines() int {
	if options.Type == "diff" {
		return 0
	}
	if options.Size != nil {
		return *options.Size
	}
	return fileDiffContextLines
}

func (options fileReturnOptions) renderFileText(operationContext context.Context, updated []byte) (string, error) {
	selection := fileTextSelection{lineRange: options.lineRange}
	preview, operationError := readFileTextPreview(operationContext, bytes.NewReader(updated), selection, true)
	if operationError != nil {
		return "", fmt.Errorf("return preview for the updated file: %w", operationError)
	}
	if preview.firstLine == 0 {
		return "Updated file is empty (0 lines).", nil
	}
	heading := fmt.Sprintf("Updated file text, lines %d-%d (absolute line-number labels are not file content):\n", preview.firstLine, preview.lastLine)
	return heading + preview.render(options.EndLine), nil
}
