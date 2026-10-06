package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Traversal uses pinned directory roots and bounded ReadDir batches. The matcher
// never gets filesystem access, so a literal prefix or brace alternative cannot
// make it follow a symlink. A directory swapped during opening is rejected.
func openGlobDirectory(operationContext context.Context, parent *os.Root, name string) (*os.Root, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	information, operationError := parent.Lstat(name)
	if operationError != nil {
		return nil, operationError
	}
	if !information.IsDir() || information.Mode()&os.ModeSymlink != 0 {
		return nil, nil
	}
	directory, operationError := parent.OpenRoot(name)
	if operationError != nil {
		return nil, operationError
	}
	openedInformation, operationError := directory.Stat(".")
	if operationError != nil {
		_ = directory.Close()
		return nil, operationError
	}
	if !os.SameFile(information, openedInformation) {
		_ = directory.Close()
		return nil, fmt.Errorf("directory %q changed during glob traversal", name)
	}
	return directory, nil
}

// The callback sees each descendant entry once. Returning false prevents descent
// into a directory; errors stop the complete search without partial output.
func walkGlobDirectory(operationContext context.Context, root *os.Root, prefix string, maximumDepth int, visit func(*os.Root, string, os.DirEntry) (bool, error)) error {
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	directory, operationError := root.Open(".")
	if operationError != nil {
		return operationError
	}
	defer directory.Close()
	for {
		entries, readError := directory.ReadDir(128)
		for _, entry := range entries {
			if operationError := operationContext.Err(); operationError != nil {
				return operationError
			}
			path := entry.Name()
			if prefix != "" {
				path = prefix + "/" + path
			}
			descend, operationError := visit(root, path, entry)
			if operationError != nil {
				return operationError
			}
			if !descend || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			if maximumDepth >= 0 && strings.Count(path, "/")+1 >= maximumDepth {
				continue
			}
			child, operationError := openGlobDirectory(operationContext, root, entry.Name())
			if operationError != nil {
				return operationError
			}
			if child == nil {
				continue
			}
			walkError := walkGlobDirectory(operationContext, child, path, maximumDepth, visit)
			closeError := child.Close()
			if operationError := errors.Join(walkError, closeError); operationError != nil {
				return operationError
			}
		}
		if errors.Is(readError, io.EOF) {
			return nil
		}
		if readError != nil {
			return readError
		}
	}
}
