package tools

import (
	"bytes"
	"fmt"
)

// Line numbers count LF-delimited lines, including a final unterminated line.
// A trailing LF does not create another line; an empty file has zero lines.
type lineRange struct {
	StartLine *int `json:"start_line"`
	EndLine   *int `json:"end_line"`
}

func (selection lineRange) specified() bool {
	return selection.StartLine != nil || selection.EndLine != nil
}

func (selection lineRange) bounds() (int, int, error) {
	start, end := 1, 0
	if selection.StartLine != nil {
		start = *selection.StartLine
		if start < 1 {
			return 0, 0, fmt.Errorf("start_line must be at least 1")
		}
	}
	if selection.EndLine != nil {
		end = *selection.EndLine
		if end < start {
			return 0, 0, fmt.Errorf("end_line must be at least start_line (%d)", start)
		}
	}
	return start, end, nil
}

// byteRange is strict for edits: a supplied endpoint must exist. Read ranges
// clamp their end at EOF instead, because reading beyond EOF cannot edit data.
func (selection lineRange) byteRange(data []byte) (int, int, error) {
	start, end, operationError := selection.bounds()
	if operationError != nil {
		return 0, 0, operationError
	}
	if !selection.specified() {
		return 0, len(data), nil
	}
	startOffset := -1
	lineNumber := 0
	for offset := 0; offset < len(data); {
		lineNumber++
		if lineNumber == start {
			startOffset = offset
		}
		lineEnd := len(data)
		if newline := bytes.IndexByte(data[offset:], '\n'); newline >= 0 {
			lineEnd = offset + newline + 1
		}
		if end != 0 && lineNumber == end {
			return startOffset, lineEnd, nil
		}
		offset = lineEnd
	}
	if startOffset < 0 {
		return 0, 0, fmt.Errorf("start_line %d exceeds the file's %d lines", start, lineNumber)
	}
	if end != 0 {
		return 0, 0, fmt.Errorf("end_line %d exceeds the file's %d lines", end, lineNumber)
	}
	return startOffset, len(data), nil
}

func closingLineBreak(data []byte) []byte {
	if bytes.HasSuffix(data, []byte("\r\n")) {
		return []byte("\r\n")
	}
	if bytes.HasSuffix(data, []byte("\n")) {
		return []byte("\n")
	}
	return nil
}
