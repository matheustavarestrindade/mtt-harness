package tools

import (
	"bytes"
	"context"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

const fileEditFeedbackDescription = " Returns the resolved path, created/updated/unchanged status, before/after line and byte counts, and a unified diff preview with absolute line ranges and 3 context lines. The preview is limited to 200 lines or 16 KiB; changed regions above 256 KiB or 4000 combined lines, and non-text data, receive an omission notice. Preview limits never limit the edit. Failures identify the operation and cause without applying an edit."

func fileEditFailure(call atom.ToolCall, toolName, path string, cause error) (atom.ToolResult, error) {
	return failedFileOperation(call, path, toolName, cause, nil)
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
