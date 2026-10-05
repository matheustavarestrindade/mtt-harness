package tools

import (
	"context"
	"fmt"
	"io"
	"os"
)

func openFileActionReader(path string) (*os.File, os.FileInfo, error) {
	information, operationError := os.Stat(path)
	if operationError != nil {
		return nil, information, operationError
	}
	if !information.Mode().IsRegular() {
		return nil, information, fmt.Errorf("target is not a regular file")
	}
	file, operationError := os.Open(path)
	return file, information, operationError
}

func (fileTool FileActions) runFileInspections(operationContext context.Context, path string, input fileActionsInput, snapshot *fileActionSnapshot) (string, error) {
	results := newFileActionResults()
	var file *os.File
	if !input.directory {
		var operationError error
		file, snapshot.information, operationError = openFileActionReader(path)
		if operationError != nil {
			return "", &fileActionFailure{index: 1, operation: input.Actions[0].Operation, cause: operationError}
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
			return "", &fileActionFailure{operation: "return read", cause: operationError}
		}
		text, operationError := results.read(operationContext, file, input.Return.fileTextSelection, fileTool.lineNumbers)
		if operationError != nil {
			return "", &fileActionFailure{operation: "return read", cause: operationError}
		}
		results.add("Return read:", text)
	case "list":
		text, operationError := results.list(operationContext, path, input.Return.Limit, input.Return.Cursor)
		if operationError != nil {
			return "", &fileActionFailure{operation: "return list", cause: operationError}
		}
		results.add("Return list:", text)
	}
	return fmt.Sprintf("Inspected %q; %d actions completed.\n\n%s", path, len(input.Actions), results.text()), nil
}
