package tools

import (
	"bytes"
	"fmt"
)

func writeFileContent(original []byte, content string, selection lineRange) ([]byte, string, error) {
	if !selection.hasBounds() {
		return []byte(content), "Whole-file write.", nil
	}
	start, end, operationError := selection.resolveByteRange(original)
	if operationError != nil {
		return nil, "", operationError
	}
	replacement := []byte(content)
	if len(replacement) > 0 && !bytes.HasSuffix(replacement, []byte("\n")) {
		replacement = append(replacement, closingLineBreak(original[start:end])...)
	}
	updated := make([]byte, 0, start+len(replacement)+len(original)-end)
	updated = append(updated, original[:start]...)
	updated = append(updated, replacement...)
	updated = append(updated, original[end:]...)
	return updated, fmt.Sprintf("Replaced lines %d-%d of the current file.", countFileLines(original[:start])+1, countFileLines(original[:end])), nil
}

func replaceFileText(original []byte, oldText, newText, mode string, selection lineRange) ([]byte, int, error) {
	start, end, operationError := selection.resolveByteRange(original)
	if operationError != nil {
		return nil, 0, operationError
	}
	selected := original[start:end]
	oldBytes, newBytes := []byte(oldText), []byte(newText)
	count := bytes.Count(selected, oldBytes)
	if count == 0 {
		return nil, 0, fmt.Errorf("old_text was not found in the selected range; the file currently has %d lines", countFileLines(original))
	}
	var replacement []byte
	if mode == "last" {
		index := bytes.LastIndex(selected, oldBytes)
		replacement = append(replacement, selected[:index]...)
		replacement = append(replacement, newBytes...)
		replacement = append(replacement, selected[index+len(oldBytes):]...)
		count = 1
	} else {
		limit := -1
		if mode == "first" {
			limit, count = 1, 1
		}
		replacement = bytes.Replace(selected, oldBytes, newBytes, limit)
	}
	updated := make([]byte, 0, start+len(replacement)+len(original)-end)
	updated = append(updated, original[:start]...)
	updated = append(updated, replacement...)
	updated = append(updated, original[end:]...)
	return updated, count, nil
}
