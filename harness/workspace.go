package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WorkspacePath is shared by checks and filesystem tools. A missing workspace
// never silently falls back to the server's current directory.
func WorkspacePath(operationContext context.Context, path string) (string, error) {
	workspace, found := WorkspaceFrom(operationContext)
	if !found {
		return "", fmt.Errorf("operation has no instance workspace")
	}
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workspace, path)
	}
	return ResolvePath(path)
}

// ResolvePath resolves existing symlink ancestors even when the final file
// does not yet exist. Broken symlinks and permission errors fail the check.
func ResolvePath(path string) (string, error) {
	absolute, operationError := filepath.Abs(path)
	if operationError != nil {
		return "", operationError
	}
	current := absolute
	var suffix []string
	for {
		resolved, operationError := filepath.EvalSymlinks(current)
		if operationError == nil {
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			return resolved, nil
		}
		if !os.IsNotExist(operationError) {
			return "", operationError
		}
		if _, linkError := os.Lstat(current); linkError == nil {
			return "", operationError
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", operationError
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}
