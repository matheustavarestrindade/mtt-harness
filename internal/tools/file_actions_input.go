package tools

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
)

const fileActionsMaximum = 32
const directoryPreviewDefaultEntries = 100

type fileAction struct {
	Operation string  `json:"op"`
	Content   *string `json:"content,omitempty"`
	OldText   *string `json:"old_text,omitempty"`
	NewText   *string `json:"new_text,omitempty"`
	Mode      *string `json:"mode,omitempty"`
	Limit     *int    `json:"limit,omitempty"`
	Cursor    string  `json:"cursor,omitempty"`
	fileTextSelection
}

type fileActionOutput struct {
	Type         string `json:"type"`
	ContextLines *int   `json:"context_lines,omitempty"`
	Limit        *int   `json:"limit,omitempty"`
	Cursor       string `json:"cursor,omitempty"`
	fileTextSelection
}

type fileActionsInput struct {
	Path      string
	Actions   []fileAction
	Return    fileActionOutput
	OnError   fileActionOutput
	mutates   bool
	directory bool
}

type fileActionFailure struct {
	index     int
	operation string
	cause     error
}

func (failure *fileActionFailure) Error() string {
	return fmt.Sprintf("action %d (%s): %v", failure.index, failure.operation, failure.cause)
}

func (failure *fileActionFailure) Unwrap() error { return failure.cause }

func decodeFileActionsInput(data []byte) (fileActionsInput, error) {
	var envelope struct {
		Path    string            `json:"path"`
		Actions []json.RawMessage `json:"actions"`
		Return  json.RawMessage   `json:"return"`
		OnError json.RawMessage   `json:"on_error"`
	}
	if operationError := decodeFileActionObject(data, &envelope, []string{"path", "actions", "return", "on_error"}); operationError != nil {
		return fileActionsInput{}, operationError
	}
	input := fileActionsInput{Path: envelope.Path}
	if input.Path == "" || len(envelope.Actions) < 1 || len(envelope.Actions) > fileActionsMaximum {
		return input, fmt.Errorf("path and 1-%d actions are required", fileActionsMaximum)
	}
	for index, encoded := range envelope.Actions {
		action, operationError := decodeFileAction(encoded)
		if operationError != nil {
			return input, &fileActionFailure{index: index + 1, operation: action.Operation, cause: operationError}
		}
		input.Actions = append(input.Actions, action)
		input.mutates = input.mutates || action.mutatesFile()
		input.directory = input.directory || action.Operation == "list"
	}
	if input.directory {
		for _, action := range input.Actions {
			if action.Operation != "list" {
				return input, fmt.Errorf("one call has one target: list actions cannot be mixed with file actions")
			}
		}
	}
	if len(envelope.Return) > 0 {
		output, operationError := decodeFileActionOutput(envelope.Return, false)
		if operationError != nil {
			return input, fmt.Errorf("return: %w", operationError)
		}
		input.Return = output
	}
	if input.mutates && input.Return.Type == "" {
		return input, fmt.Errorf("edits require an explicit return: choose summary, diff, or read; a diff is never automatic")
	}
	if input.Return.Type == "diff" && !input.mutates {
		return input, fmt.Errorf("return.type diff requires at least one mutation action")
	}
	if input.Return.Type == "list" && !input.directory || input.Return.Type == "read" && input.directory {
		return input, fmt.Errorf("the return type must match the target: read for a file, list for a directory")
	}
	if len(envelope.OnError) > 0 {
		var recovery struct {
			Return json.RawMessage `json:"return"`
		}
		if operationError := decodeFileActionObject(envelope.OnError, &recovery, []string{"return"}); operationError != nil {
			return input, fmt.Errorf("on_error: %w", operationError)
		}
		output, operationError := decodeFileActionOutput(recovery.Return, true)
		if operationError != nil {
			return input, fmt.Errorf("on_error.return: %w", operationError)
		}
		input.OnError = output
	}
	return input, nil
}

func decodeFileAction(data []byte) (fileAction, error) {
	var action fileAction
	if operationError := json.Unmarshal(data, &action); operationError != nil {
		return action, operationError
	}
	allowed := []string{"op"}
	switch action.Operation {
	case "read":
		allowed = append(allowed, "start_line", "end_line", "start_byte")
	case "write":
		allowed = append(allowed, "content", "start_line", "end_line")
	case "replace":
		allowed = append(allowed, "old_text", "new_text", "mode", "start_line", "end_line")
	case "append", "prepend":
		allowed = append(allowed, "content")
	case "list":
		allowed = append(allowed, "limit", "cursor")
	default:
		return action, fmt.Errorf("op must be read, write, replace, append, prepend, or list")
	}
	if operationError := decodeFileActionObject(data, &action, allowed); operationError != nil {
		return action, operationError
	}
	if _, _, operationError := action.resolveLineBounds(); operationError != nil {
		return action, operationError
	}
	if action.StartByte < 0 {
		return action, fmt.Errorf("start_byte must be zero or greater")
	}
	if action.Operation == "write" || action.Operation == "append" || action.Operation == "prepend" {
		if action.Content == nil {
			return action, fmt.Errorf("content is required, including an empty string for deletion or an empty file")
		}
	}
	if action.Operation == "replace" {
		if action.OldText == nil || *action.OldText == "" || action.NewText == nil {
			return action, fmt.Errorf("replace requires non-empty old_text and a new_text string")
		}
		if action.Mode != nil && !slices.Contains([]string{"first", "last", "all"}, *action.Mode) {
			return action, fmt.Errorf("mode must be first, last, or all")
		}
	}
	if action.Operation == "list" {
		if operationError := validateDirectoryPreview(action.Limit, action.Cursor); operationError != nil {
			return action, operationError
		}
	}
	return action, nil
}

func decodeFileActionOutput(data []byte, recovery bool) (fileActionOutput, error) {
	var output fileActionOutput
	if operationError := json.Unmarshal(data, &output); operationError != nil {
		return output, operationError
	}
	allowed := []string{"type"}
	switch output.Type {
	case "summary":
	case "diff":
		allowed = append(allowed, "context_lines")
	case "read":
		allowed = append(allowed, "start_line", "end_line", "start_byte")
	case "list":
		allowed = append(allowed, "limit", "cursor")
	default:
		return output, fmt.Errorf("type must be summary, diff, read, or list")
	}
	if operationError := decodeFileActionObject(data, &output, allowed); operationError != nil {
		return output, operationError
	}
	if recovery && output.Type != "read" && output.Type != "list" {
		return output, fmt.Errorf("error recovery can only read or list the same path; it cannot mutate or retry")
	}
	if output.ContextLines != nil && (*output.ContextLines < 0 || *output.ContextLines > 100) {
		return output, fmt.Errorf("context_lines must be between 0 and 100")
	}
	if _, _, operationError := output.resolveLineBounds(); operationError != nil {
		return output, operationError
	}
	if output.StartByte < 0 {
		return output, fmt.Errorf("start_byte must be zero or greater")
	}
	if output.Type == "list" {
		return output, validateDirectoryPreview(output.Limit, output.Cursor)
	}
	return output, nil
}

func decodeFileActionObject(data []byte, destination any, allowed []string) error {
	var fields map[string]json.RawMessage
	if operationError := json.Unmarshal(data, &fields); operationError != nil {
		return operationError
	}
	if fields == nil {
		return fmt.Errorf("expected an object, not null")
	}
	for name, value := range fields {
		if !slices.Contains(allowed, name) {
			return fmt.Errorf("%s is not valid for this operation", name)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("%s cannot be null; omit optional fields instead", name)
		}
	}
	return json.Unmarshal(data, destination)
}

func validateDirectoryPreview(limit *int, cursor string) error {
	if limit != nil && (*limit < 1 || *limit > filePreviewMaxLines) {
		return fmt.Errorf("list limit must be between 1 and %d entries", filePreviewMaxLines)
	}
	if len(cursor) > 512 {
		return fmt.Errorf("invalid directory cursor; use the cursor returned by list")
	}
	if _, operationError := base64.RawURLEncoding.DecodeString(cursor); operationError != nil {
		return fmt.Errorf("invalid directory cursor: %w", operationError)
	}
	return nil
}

func (action fileAction) mutatesFile() bool {
	return action.Operation == "write" || action.Operation == "replace" || action.Operation == "append" || action.Operation == "prepend"
}
