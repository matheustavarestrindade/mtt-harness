package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
)

func openFileActionReader(path string) (*os.File, error) {
	information, operationError := os.Stat(path)
	if operationError != nil {
		return nil, operationError
	}
	if !information.Mode().IsRegular() {
		return nil, fmt.Errorf("read requires a regular file; use list for a directory")
	}
	return os.Open(path)
}

func (fileTool FileActions) runFileInspections(operationContext context.Context, path string, input fileActionsInput) (string, error) {
	results := newFileActionResults()
	var file *os.File
	if !input.directory {
		var operationError error
		file, operationError = openFileActionReader(path)
		if operationError != nil {
			return "", operationError
		}
		defer file.Close()
	}
	for index, action := range input.Actions {
		var text string
		var operationError error
		if action.Operation == "list" {
			text, operationError = results.list(operationContext, path, action.Limit, action.Cursor)
		} else {
			if _, operationError = file.Seek(0, io.SeekStart); operationError == nil {
				text, operationError = results.read(operationContext, file, action.fileTextSelection, fileTool.lineNumbers)
			}
		}
		if operationError != nil {
			return "", &fileActionFailure{index: index + 1, operation: action.Operation, cause: operationError}
		}
		if len(input.Actions) == 1 && input.Return.Type == "" {
			return text, nil
		}
		results.add(fmt.Sprintf("Action %d %s:", index+1, action.Operation), text)
	}
	switch input.Return.Type {
	case "read":
		if _, operationError := file.Seek(0, io.SeekStart); operationError != nil {
			return "", operationError
		}
		text, operationError := results.read(operationContext, file, input.Return.fileTextSelection, fileTool.lineNumbers)
		if operationError != nil {
			return "", fmt.Errorf("return read: %w", operationError)
		}
		results.add("Return read:", text)
	case "list":
		text, operationError := results.list(operationContext, path, input.Return.Limit, input.Return.Cursor)
		if operationError != nil {
			return "", fmt.Errorf("return list: %w", operationError)
		}
		results.add("Return list:", text)
	}
	return fmt.Sprintf("Inspected %q; %d actions completed.\n\n%s", path, len(input.Actions), results.text()), nil
}

func (fileTool FileActions) renderFileActionRecovery(operationContext context.Context, path string, output fileActionOutput, snapshot fileActionSnapshot) string {
	results := newFileActionResults()
	var text string
	var operationError error
	switch output.Type {
	case "list":
		text, operationError = results.list(operationContext, path, output.Limit, output.Cursor)
	case "read":
		if snapshot.loaded {
			if !snapshot.exists {
				operationError = fmt.Errorf("the original file did not exist")
			} else {
				text, operationError = results.read(operationContext, bytes.NewReader(snapshot.content), output.fileTextSelection, fileTool.lineNumbers)
			}
			break
		}
		var file *os.File
		file, operationError = openFileActionReader(path)
		if operationError == nil {
			defer file.Close()
			text, operationError = results.read(operationContext, file, output.fileTextSelection, fileTool.lineNumbers)
		}
	}
	if operationError != nil {
		return fmt.Sprintf("Recovery %s preview unavailable: %v. The original failure is preserved below.", output.Type, operationError)
	}
	return "Recovery " + output.Type + " preview (no edits from this call were committed):\n" + text
}
