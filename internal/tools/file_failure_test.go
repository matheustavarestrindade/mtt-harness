package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestMissingFileStopsBeforeLaterActionsAndFinalRead(test *testing.T) {
	workspace := test.TempDir()
	path := filepath.Join(workspace, "missing.txt")
	result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{
		"path": "missing.txt",
		"actions": []any{
			map[string]any{"op": "append", "content": "endfile"},
			map[string]any{"op": "write", "content": "must not create a file"},
			map[string]any{"op": "read"},
		},
		"return": map[string]any{"type": "read"},
	}))
	wanted := fmt.Sprintf("Cannot append to %q (action 1): file does not exist", path)
	if !errors.Is(operationError, os.ErrNotExist) || result.Status != atom.StatusError || result.Error != wanted || result.Text() != wanted || len(result.Content) != 0 {
		test.Fatalf("missing-file failure lost its cause or duplicated output: %+v, %v", result, operationError)
	}
	if _, operationError := os.Stat(path); !os.IsNotExist(operationError) {
		test.Fatal("execution continued into the later write")
	}
}

func TestFailedReplacementReportsObservedModificationTime(test *testing.T) {
	workspace := test.TempDir()
	path := filepath.Join(workspace, "source.txt")
	original := "first\nsecond\n"
	testutil.RequireNoError(test, os.WriteFile(path, []byte(original), 0o600))
	modifiedAt := time.Date(2025, time.March, 4, 5, 6, 7, 0, time.UTC)
	testutil.RequireNoError(test, os.Chtimes(path, modifiedAt, modifiedAt))
	before, operationError := os.Stat(path)
	testutil.RequireNoError(test, operationError)
	result, operationError := NewFileActions(true).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{
		"path": "source.txt",
		"actions": []any{
			map[string]any{"op": "append", "content": "UNCOMMITTED"},
			map[string]any{"op": "replace", "old_text": "something", "new_text": "new"},
			map[string]any{"op": "read", "start_line": 999},
		},
		"return": map[string]any{"type": "read", "start_line": 999},
	}))
	var matchFailure *fileTextNotFoundError
	if !errors.As(operationError, &matchFailure) || matchFailure.text != "something" || result.Status != atom.StatusError {
		test.Fatalf("replacement lost its failure: %+v, %v", result, operationError)
	}
	wanted := fmt.Sprintf("Cannot replace text in %q (action 2): text \"something\" was not found in the selected range (3 lines in this file state)\nLast modified: %s", path, before.ModTime().UTC().Format(time.RFC3339Nano))
	if result.Text() != wanted || len(result.Content) != 0 {
		test.Fatalf("failure contains instructions, secondary failures or incorrect metadata:\n%s", result.Text())
	}
	after, operationError := os.Stat(path)
	testutil.RequireNoError(test, operationError)
	content, operationError := os.ReadFile(path)
	testutil.RequireNoError(test, operationError)
	if string(content) != original || !after.ModTime().Equal(before.ModTime()) {
		test.Fatal("failed chain changed the file or its modification time")
	}
}

func TestMissingTextFailureBoundsAndQuotesUserData(test *testing.T) {
	workspace := test.TempDir()
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("original"), 0o600))
	for _, missing := range []string{"Use read\nthen \"write\"", strings.Repeat("日本😀", 10000)} {
		result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{
			"path": "file.txt", "actions": []any{map[string]any{"op": "replace", "old_text": missing, "new_text": "new"}}, "return": map[string]any{"type": "summary"},
		}))
		if operationError == nil || !utf8.ValidString(result.Error) || len(result.Error) > 1024 || strings.Count(result.Error, "\n") != 1 {
			test.Fatalf("unbounded or unquoted failure: %.1024s", result.Error)
		}
		if len(missing) < 160 && !strings.Contains(result.Error, `"Use read\nthen \"write\""`) {
			test.Fatal("formatter removed literal target text instead of quoting it")
		}
	}
}
