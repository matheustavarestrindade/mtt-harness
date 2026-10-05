package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type fileActionFailure struct {
	index     int
	operation string
	cause     error
}

func (failure *fileActionFailure) Error() string {
	return fmt.Sprintf("action %d (%s): %v", failure.index, failure.operation, failure.cause)
}

func (failure *fileActionFailure) Unwrap() error { return failure.cause }

type fileTextNotFoundError struct {
	text      string
	lineCount int
}

func (failure *fileTextNotFoundError) Error() string {
	text := failure.text
	const maximumTextBytes = 160
	if len(text) > maximumTextBytes {
		text = string(completeUTF8PreviewPrefix([]byte(text[:maximumTextBytes])))
		return fmt.Sprintf("text %s… (%d bytes) was not found in the selected range (%d lines in this file state)", strconv.Quote(text), len(failure.text), failure.lineCount)
	}
	return fmt.Sprintf("text %s was not found in the selected range (%d lines in this file state)", strconv.Quote(text), failure.lineCount)
}

// A failure reports only the operation that stopped and facts observed before
// it stopped. Its cause stays available to Go callers without repeating every
// wrapper in the model-facing error. Formatting performs no recovery I/O.
type fileToolError struct {
	path        string
	operation   string
	actionIndex int
	cause       error
	modifiedAt  time.Time
}

func (failure *fileToolError) Unwrap() error { return failure.cause }

func (failure *fileToolError) Error() string {
	operation := "complete file_actions for"
	switch failure.operation {
	case "read", "write", "list", "delete":
		operation = failure.operation
	case "replace":
		operation = "replace text in"
	case "append", "prepend":
		operation = failure.operation + " to"
	case "return read":
		operation = "produce the final read of"
	case "return list":
		operation = "produce the final listing of"
	case "return diff":
		operation = "produce the final diff of"
	case "commit":
		operation = "commit changes to"
	}
	var output strings.Builder
	if failure.path == "" {
		output.WriteString("Invalid file request")
	} else {
		fmt.Fprintf(&output, "Cannot %s %q", operation, failure.path)
	}
	if failure.actionIndex > 0 {
		fmt.Fprintf(&output, " (action %d)", failure.actionIndex)
	}
	fmt.Fprintf(&output, ": %s", fileFailureReason(failure.cause, failure.operation))
	if !failure.modifiedAt.IsZero() {
		fmt.Fprintf(&output, "\nLast modified: %s", failure.modifiedAt.UTC().Format(time.RFC3339Nano))
	}
	return output.String()
}

func fileFailureReason(cause error, operation string) string {
	switch {
	case errors.Is(cause, os.ErrNotExist):
		if operation == "list" || operation == "return list" {
			return "directory does not exist"
		}
		return "file does not exist"
	case errors.Is(cause, os.ErrPermission):
		return "permission denied"
	case errors.Is(cause, context.Canceled):
		return "operation canceled"
	case errors.Is(cause, context.DeadlineExceeded):
		return "operation deadline exceeded"
	}
	var actionFailure *fileActionFailure
	if errors.As(cause, &actionFailure) {
		return fileFailureReason(actionFailure.cause, operation)
	}
	var pathError *os.PathError
	if errors.As(cause, &pathError) {
		return pathError.Err.Error()
	}
	return cause.Error()
}

func failedFileOperation(call atom.ToolCall, path, operation string, cause error, information os.FileInfo) (atom.ToolResult, error) {
	failure := &fileToolError{path: path, operation: operation, cause: cause}
	var actionFailure *fileActionFailure
	if errors.As(cause, &actionFailure) {
		failure.operation = actionFailure.operation
		failure.actionIndex = actionFailure.index
	}
	if information != nil {
		failure.modifiedAt = information.ModTime()
	}
	return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: failure.Error()}, failure
}
