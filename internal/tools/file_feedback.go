package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

const fileEditFeedbackDescription = " Returns the resolved path, created/updated/unchanged status, before/after line and byte counts, and a unified diff preview with absolute line ranges and 3 context lines. The preview is limited to 200 lines or 16 KiB; changed regions above 256 KiB or 4000 combined lines, and non-text data, receive an omission notice. Preview limits never limit the edit. Failures explain the cause and recovery steps without applying an edit."

func fileEditFailure(call atom.ToolCall, toolName, path string, cause error, recovery string) (atom.ToolResult, error) {
	switch {
	case errors.Is(cause, context.Canceled), errors.Is(cause, context.DeadlineExceeded):
		recovery = "The edit was cancelled. If you resume the task, use read to inspect the current file before another edit."
	case errors.Is(cause, os.ErrNotExist):
		recovery = "Verify the path and parent directory. A range write or replace requires an existing file. Use write without line bounds only if you intend to create a file."
	case errors.Is(cause, os.ErrPermission):
		recovery = "Check access to the file and its parent directory. Resolve the permission error, then use read before retrying."
	}
	operationError := fmt.Errorf("%s %q failed: %w\nThis call did not change the file.\n%s", toolName, path, cause, recovery)
	return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
}

// Feedback uses the exact snapshots owned by the edit gate, never a later read
// that could describe a different writer's changes. Reporting cannot fail an
// edit after its rename has committed.
func describeFileEdit(operationContext context.Context, path string, original []byte, change fileChange, existed bool) (string, error) {
	updated := change.content
	status := "Updated"
	unchanged := bytes.Equal(original, updated)
	if !existed {
		status = "Created"
	} else if unchanged {
		status = "Unchanged"
	}
	summary := fmt.Sprintf("%s %q\n%s\nLines: %d -> %d. Bytes: %d -> %d.\n", status, path, change.summary, countFileLines(original), countFileLines(updated), len(original), len(updated))
	if change.returnOptions.Type == "lines" || change.returnOptions.Type == "file" {
		preview, operationError := change.returnOptions.renderFileText(operationContext, updated)
		if operationError != nil {
			return "", operationError
		}
		return summary + "\n" + preview, nil
	}
	if unchanged && existed {
		return summary + "No content changes; the file already contained the requested text.", nil
	}
	if unchanged {
		return summary + "Created an empty file.", nil
	}
	return summary + "\n" + renderFileEditDiff(path, original, updated, existed, change.returnOptions.diffContextLines()), nil
}

func countFileLines(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	lines := bytes.Count(content, []byte{'\n'})
	if content[len(content)-1] != '\n' {
		lines++
	}
	return lines
}
