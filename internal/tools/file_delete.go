package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func resolveFileActionPath(operationContext context.Context, input fileActionsInput) (string, error) {
	if !input.deletes {
		return harness.WorkspacePath(operationContext, input.Path)
	}
	path, operationError := harness.WorkspaceEntryPath(operationContext, input.Path)
	if operationError != nil {
		return "", operationError
	}
	workspace, _ := harness.WorkspaceFrom(operationContext)
	root, operationError := harness.ResolvePath(workspace)
	if operationError != nil {
		return "", operationError
	}
	workspacePath, operationError := filepath.Abs(workspace)
	if operationError != nil {
		return "", operationError
	}
	workspaceEntry, operationError := harness.WorkspaceEntryPath(operationContext, workspacePath)
	if operationError != nil {
		return "", operationError
	}
	if filepath.Clean(path) == filepath.Clean(root) || filepath.Clean(path) == filepath.Clean(workspaceEntry) {
		return "", fmt.Errorf("delete cannot target the instance workspace root")
	}
	return path, nil
}

// A standalone delete does not read file contents unless a diff is requested.
// os.Remove is one unlink/rmdir operation; non-empty directories stay intact.
func (fileTool FileActions) runFileDeletion(operationContext context.Context, path string, input fileActionsInput, snapshot *fileActionSnapshot) (string, error) {
	information, operationError := os.Lstat(path)
	snapshot.information = information
	if operationError != nil {
		return "", &fileActionFailure{index: 1, operation: "delete", cause: operationError}
	}
	if !information.Mode().IsRegular() && !information.IsDir() && information.Mode()&os.ModeSymlink == 0 {
		return "", &fileActionFailure{index: 1, operation: "delete", cause: fmt.Errorf("target is not a regular file, symbolic link, or empty directory")}
	}
	output := "Deleted"
	switch input.Return.Type {
	case "read":
		return "", &fileActionFailure{operation: "return read", cause: os.ErrNotExist}
	case "diff":
		if !information.Mode().IsRegular() {
			return "", &fileActionFailure{operation: "return diff", cause: fmt.Errorf("text diffs require a regular file")}
		}
		original, operationError := os.ReadFile(path)
		if operationError != nil {
			return "", &fileActionFailure{operation: "return diff", cause: operationError}
		}
		contextLines := fileDiffContextLines
		if input.Return.ContextLines != nil {
			contextLines = *input.Return.ContextLines
		}
		output = renderFileStateDiffWithin(path, original, nil, true, false, contextLines, newFileActionResults().budget)
	}
	if operationError := operationContext.Err(); operationError != nil {
		return "", &fileActionFailure{index: 1, operation: "delete", cause: operationError}
	}
	if operationError := os.Remove(path); operationError != nil {
		return "", &fileActionFailure{index: 1, operation: "delete", cause: operationError}
	}
	return output, nil
}
