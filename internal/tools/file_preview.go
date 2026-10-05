package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	filePreviewMaxLines = 200
	filePreviewMaxBytes = 16 * 1024
)

type fileTextSelection struct {
	lineRange
	StartByte int64 `json:"start_byte"`
}

type fileTextPreview struct {
	content   string
	firstLine int
	lastLine  int
	nextLine  int
	nextByte  int64
}

// A continuation cursor addresses file bytes, not display labels. Keeping the
// offset within its line permits progress even through a single enormous line.
func (preview fileTextPreview) render(endLine *int) string {
	if preview.nextLine == 0 {
		return preview.content
	}
	continuation := fmt.Sprintf(`{"start_line":%d`, preview.nextLine)
	if preview.nextByte > 0 {
		continuation += fmt.Sprintf(`,"start_byte":%d`, preview.nextByte)
	}
	if endLine != nil {
		continuation += fmt.Sprintf(`,"end_line":%d`, *endLine)
	}
	continuation += "}"
	return preview.content + "\n\n[File text preview truncated at 200 lines or 16 KiB, including display labels. Continue with read on the same path using " + continuation + ". This notice is not file content.]"
}

// readFileTextPreview buffers only the output allowance and a small reader
// window. Skipping old lines or a long-line prefix never retains their bytes.
func readFileTextPreview(operationContext context.Context, source io.Reader, selection fileTextSelection, lineNumbers bool) (fileTextPreview, error) {
	return readFileTextPreviewWithin(operationContext, source, selection, lineNumbers, filePreviewLimits{lines: filePreviewMaxLines, bytes: filePreviewMaxBytes})
}

func readFileTextPreviewWithin(operationContext context.Context, source io.Reader, selection fileTextSelection, lineNumbers bool, limits filePreviewLimits) (fileTextPreview, error) {
	startLine, endLine, operationError := selection.resolveLineBounds()
	if operationError != nil {
		return fileTextPreview{}, operationError
	}
	if selection.StartByte < 0 {
		return fileTextPreview{}, fmt.Errorf("start_byte must be zero or greater")
	}
	reader := bufio.NewReader(source)
	for lineNumber := 1; lineNumber < startLine; lineNumber++ {
		exists, operationError := discardPreviewLine(operationContext, reader)
		if operationError != nil {
			return fileTextPreview{}, operationError
		}
		if !exists {
			return fileTextPreview{}, fmt.Errorf("start_line %d exceeds the file's %d lines", startLine, lineNumber-1)
		}
	}
	preview := fileTextPreview{}
	var output strings.Builder
	for lineNumber := startLine; endLine == 0 || lineNumber <= endLine; lineNumber++ {
		if operationError := operationContext.Err(); operationError != nil {
			return fileTextPreview{}, operationError
		}
		if _, operationError := reader.Peek(1); operationError != nil {
			if !errors.Is(operationError, io.EOF) {
				return fileTextPreview{}, operationError
			}
			if lineNumber == startLine && (selection.hasBounds() || selection.StartByte > 0) {
				return fileTextPreview{}, fmt.Errorf("start_line %d exceeds the file's %d lines", startLine, lineNumber-1)
			}
			break
		}
		startByte := int64(0)
		if lineNumber == startLine {
			startByte = selection.StartByte
		}
		prefix := ""
		if lineNumbers {
			prefix = strconv.Itoa(lineNumber) + ": "
		}
		remainingBytes := limits.bytes - output.Len() - len(prefix)
		if lineNumber-startLine >= limits.lines || remainingBytes <= 0 {
			preview.nextLine, preview.nextByte = lineNumber, startByte
			break
		}
		text, truncated, operationError := readPreviewLine(operationContext, reader, startByte, remainingBytes)
		if operationError != nil {
			return fileTextPreview{}, fmt.Errorf("line %d: %w", lineNumber, operationError)
		}
		if len(text) > 0 {
			if preview.firstLine == 0 {
				preview.firstLine = lineNumber
			}
			preview.lastLine = lineNumber
			output.WriteString(prefix)
			output.Write(text)
		}
		if truncated {
			preview.nextLine, preview.nextByte = lineNumber, startByte+int64(len(text))
			break
		}
	}
	preview.content = output.String()
	return preview, nil
}

func discardPreviewLine(operationContext context.Context, reader *bufio.Reader) (bool, error) {
	exists := false
	for {
		if operationError := operationContext.Err(); operationError != nil {
			return false, operationError
		}
		fragment, readError := reader.ReadSlice('\n')
		exists = exists || len(fragment) > 0
		if errors.Is(readError, bufio.ErrBufferFull) {
			continue
		}
		if readError == nil || errors.Is(readError, io.EOF) {
			return exists, nil
		}
		return false, readError
	}
}

func readPreviewLine(operationContext context.Context, reader *bufio.Reader, startByte int64, maximumBytes int) ([]byte, bool, error) {
	var output []byte
	lineBytes := int64(0)
	for {
		if operationError := operationContext.Err(); operationError != nil {
			return nil, false, operationError
		}
		fragment, readError := reader.ReadSlice('\n')
		if readError != nil && !errors.Is(readError, bufio.ErrBufferFull) && !errors.Is(readError, io.EOF) {
			return nil, false, readError
		}
		fragmentStart := lineBytes
		lineBytes += int64(len(fragment))
		if startByte > fragmentStart {
			skipped := min(startByte-fragmentStart, int64(len(fragment)))
			fragment = fragment[int(skipped):]
		}
		if len(output) == 0 && len(fragment) > 0 && startByte > 0 && !utf8.RuneStart(fragment[0]) {
			return nil, false, fmt.Errorf("start_byte %d splits a UTF-8 character; use the exact offset from the previous truncation notice", startByte)
		}
		available := maximumBytes - len(output)
		if len(fragment) > available {
			output = append(output, fragment[:available]...)
			return completeUTF8PreviewPrefix(output), true, nil
		}
		output = append(output, fragment...)
		if !errors.Is(readError, bufio.ErrBufferFull) {
			if startByte > 0 && startByte >= lineBytes {
				return nil, false, fmt.Errorf("start_byte %d is outside this line's %d bytes; use the continuation cursor or start_byte 0", startByte, lineBytes)
			}
			return output, false, nil
		}
		if len(output) == maximumBytes {
			if _, operationError := reader.Peek(1); operationError != nil {
				if errors.Is(operationError, io.EOF) {
					return output, false, nil
				}
				return nil, false, operationError
			}
			return completeUTF8PreviewPrefix(output), true, nil
		}
	}
}

// A cap must not split an otherwise valid final rune. Invalid bytes already in
// the source are not silently deleted or mistaken for a continuation offset.
func completeUTF8PreviewPrefix(content []byte) []byte {
	boundary := len(content)
	for boundary > 0 && !utf8.RuneStart(content[boundary-1]) {
		boundary--
	}
	if boundary > 0 && !utf8.FullRune(content[boundary-1:]) {
		return content[:boundary-1]
	}
	return content
}
