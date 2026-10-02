package tools

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type fileChange struct {
	content []byte
	summary string
}

type fileEditGate struct {
	turn  chan struct{}
	users int
}

var fileEdits = struct {
	mutex sync.Mutex
	paths map[string]*fileEditGate
}{paths: make(map[string]*fileEditGate)}

// The short registry lock only owns gate references. File I/O happens after it
// is released. Waiters count as users so a gate cannot be replaced beneath them.
func acquireFileEdit(operationContext context.Context, path string) (func(), error) {
	fileEdits.mutex.Lock()
	gate := fileEdits.paths[path]
	if gate == nil {
		gate = &fileEditGate{turn: make(chan struct{}, 1)}
		fileEdits.paths[path] = gate
	}
	gate.users++
	fileEdits.mutex.Unlock()
	releaseReference := func() {
		fileEdits.mutex.Lock()
		defer fileEdits.mutex.Unlock()
		gate.users--
		if gate.users == 0 {
			delete(fileEdits.paths, path)
		}
	}
	select {
	case gate.turn <- struct{}{}:
		return func() {
			<-gate.turn
			releaseReference()
		}, nil
	case <-operationContext.Done():
		releaseReference()
		return nil, operationContext.Err()
	}
}

// applyAtomicFileEdit serializes harness edits to the same resolved path. A replacement
// is prepared before rename so invalid ranges, absent matches, and failed writes
// leave the original file intact. External editors do not participate in this gate.
func applyAtomicFileEdit(operationContext context.Context, path string, allowCreate bool, transform func([]byte) (fileChange, error)) (string, error) {
	release, operationError := acquireFileEdit(operationContext, path)
	if operationError != nil {
		return "", operationError
	}
	defer release()
	if operationError := operationContext.Err(); operationError != nil {
		return "", operationError
	}
	information, operationError := os.Stat(path)
	if operationError != nil && !(allowCreate && os.IsNotExist(operationError)) {
		return "", operationError
	}
	var original []byte
	if information != nil {
		if !information.Mode().IsRegular() {
			return "", fmt.Errorf("file edits require a regular file")
		}
		original, operationError = os.ReadFile(path)
		if operationError != nil {
			return "", operationError
		}
	}
	change, operationError := transform(original)
	if operationError != nil {
		return "", operationError
	}
	if operationError := operationContext.Err(); operationError != nil {
		return "", operationError
	}
	if operationError := replaceFileContentsAtomically(operationContext, path, information, change.content); operationError != nil {
		return "", operationError
	}
	return describeFileEdit(path, original, change.content, information != nil, change.summary), nil
}

func replaceFileContentsAtomically(operationContext context.Context, path string, information os.FileInfo, content []byte) error {
	permissions := os.FileMode(0o644)
	if information != nil {
		// Rename permission alone must not bypass a read-only destination.
		destination, operationError := os.OpenFile(path, os.O_WRONLY, 0)
		if operationError != nil {
			return operationError
		}
		if operationError := destination.Close(); operationError != nil {
			return operationError
		}
		permissions = information.Mode().Perm()
	}
	temporaryPath := filepath.Join(filepath.Dir(path), ".mtt-edit-"+rand.Text())
	temporaryFile, operationError := os.OpenFile(temporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, permissions)
	if operationError != nil {
		return operationError
	}
	defer os.Remove(temporaryPath)
	if information != nil {
		if operationError := temporaryFile.Chmod(permissions); operationError != nil {
			return errors.Join(operationError, temporaryFile.Close())
		}
	}
	_, writeError := temporaryFile.Write(content)
	closeError := temporaryFile.Close()
	if operationError := errors.Join(writeError, closeError); operationError != nil {
		return operationError
	}
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	return os.Rename(temporaryPath, path)
}
