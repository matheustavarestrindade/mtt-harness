//go:build !unix

package workspaceread

import (
	"fmt"
	"os"
)

func openEntry(root *os.Root, path string) (*os.File, error) {
	metadata, operationError := root.Lstat(path)
	if operationError != nil {
		return nil, operationError
	}
	if metadata.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("workspace entry is a symbolic link")
	}
	return root.Open(path)
}
