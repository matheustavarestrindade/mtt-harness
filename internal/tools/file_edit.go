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
	content       []byte
	summary       string
	returnOptions fileReturnOptions
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
	// Prepare the bounded return value before commit. Invalid post-write ranges
	// cannot produce a failed call after changing the file, and no later writer
	// can replace the bytes used by this response.
	feedback, operationError := describeFileEdit(operationContext, path, original, change, information != nil)
	if operationError != nil {
		return "", operationError
	}
	if operationError := operationContext.Err(); operationError != nil {
		return "", operationError
	}
	if operationError := replaceFileContentsAtomically(operationContext, path, information, change.content); operationError != nil {
		return "", operationError
	}
	return feedback, nil
}

func replaceFileContentsAtomically(operationContext context.Context, path string, information os.FileInfo, content []byte) error {
	replacement, operationError := prepareFileReplacement(operationContext, path, information, content)
	if operationError != nil {
		return operationError
	}
	defer replacement.discard()
	return replacement.commit(operationContext)
}

type preparedFileReplacement struct{ path, temporaryPath string }

func (replacement *preparedFileReplacement) discard() { _ = os.Remove(replacement.temporaryPath) }

func (replacement *preparedFileReplacement) commit(operationContext context.Context) error {
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	return os.Rename(replacement.temporaryPath, replacement.path)
}

// Prepare every replacement before a multi-path call commits its first file.
// Preparation failures leave destinations intact and remove their temporary data.
func prepareFileReplacement(operationContext context.Context, path string, information os.FileInfo, content []byte) (*preparedFileReplacement, error) {
	permissions := os.FileMode(0o644)
	if information != nil {
		// Rename permission alone must not bypass a read-only destination.
		destination, operationError := os.OpenFile(path, os.O_WRONLY, 0)
		if operationError != nil {
			return nil, operationError
		}
		if operationError := destination.Close(); operationError != nil {
			return nil, operationError
		}
		permissions = information.Mode().Perm()
	}
	temporaryPath := filepath.Join(filepath.Dir(path), ".mtt-edit-"+rand.Text())
	temporaryFile, operationError := os.OpenFile(temporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, permissions)
	if operationError != nil {
		return nil, operationError
	}
	prepared := false
	defer func() {
		if !prepared {
			_ = os.Remove(temporaryPath)
		}
	}()
	if information != nil {
		if operationError := temporaryFile.Chmod(permissions); operationError != nil {
			return nil, errors.Join(operationError, temporaryFile.Close())
		}
	}
	_, writeError := temporaryFile.Write(content)
	closeError := temporaryFile.Close()
	if operationError := errors.Join(writeError, closeError); operationError != nil {
		return nil, operationError
	}
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	prepared = true
	return &preparedFileReplacement{path: path, temporaryPath: temporaryPath}, nil
}
