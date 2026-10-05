package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	inputSchemaValidation "github.com/matheustavarestrindade/mtt-harness/internal/molecule/schema"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestFileActionsStageAnOrderedChainAndReturnChosenText(test *testing.T) {
	workspace := test.TempDir()
	arguments := map[string]any{
		"path": "source.txt",
		"actions": []any{
			map[string]any{"op": "write", "content": "first\nold\n"},
			map[string]any{"op": "replace", "old_text": "old", "new_text": "new"},
			map[string]any{"op": "append", "content": "tail"},
			map[string]any{"op": "prepend", "content": "top\n"},
			map[string]any{"op": "write", "start_line": 2, "end_line": 2, "content": "changed"},
		},
		"return": map[string]any{"type": "read", "start_line": 2, "end_line": 99},
	}
	call := fileCall(test, arguments)
	fileTool := NewFileActions(true)
	testutil.RequireNoError(test, inputSchemaValidation.Validate(fileTool.InputSchema().JSON, call.Input))
	result, operationError := fileTool.Run(harness.WithWorkspace(context.Background(), workspace), call)
	testutil.RequireNoError(test, operationError)
	actual, operationError := os.ReadFile(filepath.Join(workspace, "source.txt"))
	testutil.RequireNoError(test, operationError)
	if string(actual) != "top\nchanged\nnew\ntail" || result.Text() != "2: changed\n3: new\n4: tail" {
		test.Fatalf("incorrect ordered edit or selected output: %q\n%s", actual, result.Text())
	}
}

func TestFileActionsNeverChooseADiffForTheModel(test *testing.T) {
	workspace := test.TempDir()
	operationContext := harness.WithWorkspace(context.Background(), workspace)
	arguments := map[string]any{"path": "source.txt", "actions": []any{map[string]any{"op": "write", "content": "private file content"}}}
	result, operationError := NewFileActions(false).Run(operationContext, fileCall(test, arguments))
	if operationError == nil || !strings.Contains(result.Error, "explicit return") {
		test.Fatal("mutation did not require an output choice")
	}
	if _, operationError := os.Stat(filepath.Join(workspace, "source.txt")); !os.IsNotExist(operationError) {
		test.Fatal("missing return created a file")
	}
	arguments["return"] = map[string]any{"type": "summary"}
	result, operationError = NewFileActions(false).Run(operationContext, fileCall(test, arguments))
	testutil.RequireNoError(test, operationError)
	if result.Text() != "Created" {
		test.Fatalf("summary leaked unsolicited file content: %s", result.Text())
	}
}

func TestFileActionsStopAtFailureAndDiscardStagedOutput(test *testing.T) {
	workspace := test.TempDir()
	path := filepath.Join(workspace, "source.txt")
	original := "original\nsecond\n"
	testutil.RequireNoError(test, os.WriteFile(path, []byte(original), 0o600))
	result, operationError := NewFileActions(true).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{
		"path": "source.txt",
		"actions": []any{
			map[string]any{"op": "write", "content": "UNCOMMITTED\nsecond\n"},
			map[string]any{"op": "read"},
			map[string]any{"op": "replace", "old_text": "missing", "new_text": "new"},
			map[string]any{"op": "read", "start_line": 999},
			map[string]any{"op": "write", "content": "MUST NOT RUN"},
		},
		"return": map[string]any{"type": "read", "start_line": 999},
	}))
	if operationError == nil || result.Status != atom.StatusError || !strings.Contains(result.Error, "action 3") || !strings.Contains(result.Error, `text "missing" was not found`) {
		test.Fatalf("failed edit lost status/cause: %+v %v", result, operationError)
	}
	if len(result.Content) != 0 || result.Text() != result.Error || strings.Contains(result.Text(), "UNCOMMITTED") || strings.Contains(result.Text(), "999") || strings.Contains(result.Text(), "MUST NOT RUN") {
		test.Fatalf("failed call returned other action output: %s", result.Text())
	}
	actual, operationError := os.ReadFile(path)
	testutil.RequireNoError(test, operationError)
	if string(actual) != original {
		test.Fatal("failed chain partially committed")
	}
}

func TestFileActionsInvalidReturnOrLateFailureCannotCreateAFile(test *testing.T) {
	for _, output := range []map[string]any{{"type": "read", "start_line": 20}, {"type": "summary"}} {
		workspace := test.TempDir()
		actions := []any{map[string]any{"op": "write", "content": "new"}}
		if output["type"] == "summary" {
			actions = append(actions, map[string]any{"op": "replace", "old_text": "missing", "new_text": "x"})
		}
		_, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": "new.txt", "actions": actions, "return": output}))
		if operationError == nil {
			test.Fatal("invalid operation succeeded")
		}
		entries, operationError := os.ReadDir(workspace)
		testutil.RequireNoError(test, operationError)
		if len(entries) != 0 {
			test.Fatalf("failed chain created a destination or temporary file: %v", entries)
		}
	}
}

func TestFileActionsRejectErrorRecoveryBeforeExecution(test *testing.T) {
	workspace := test.TempDir()
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "source.txt"), []byte("old"), 0o600))
	result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{
		"path": "source.txt", "actions": []any{map[string]any{"op": "append", "content": "changed"}},
		"return": map[string]any{"type": "summary"}, "on_error": map[string]any{"return": map[string]any{"type": "read", "start_line": 20}},
	}))
	if operationError == nil || !strings.Contains(result.Error, "on_error is not valid") || len(result.Content) != 0 || result.Status != atom.StatusError {
		test.Fatalf("unsupported recovery accepted: %+v", result)
	}
	actual, operationError := os.ReadFile(filepath.Join(workspace, "source.txt"))
	testutil.RequireNoError(test, operationError)
	if string(actual) != "old" {
		test.Fatal("unsupported input ran an action")
	}
}

func TestFileActionsShareOnePreviewBudget(test *testing.T) {
	workspace := test.TempDir()
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "source.txt"), []byte(strings.Repeat("ROW_MARKER\n", 200)), 0o600))
	result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{
		"path": "source.txt", "actions": []any{
			map[string]any{"op": "read", "end_line": 150}, map[string]any{"op": "read", "end_line": 150},
		},
	}))
	testutil.RequireNoError(test, operationError)
	if strings.Count(result.Text(), "ROW_MARKER") != 200 || !strings.Contains(result.Text(), `"start_line":51`) {
		test.Fatalf("read actions bypassed the shared cap: %s", result.Text())
	}
}

func TestConcurrentFileActionAppendsDoNotLoseUpdates(test *testing.T) {
	workspace := test.TempDir()
	path := filepath.Join(workspace, "source.txt")
	testutil.RequireNoError(test, os.WriteFile(path, nil, 0o600))
	operationContext := harness.WithWorkspace(context.Background(), workspace)
	var workers sync.WaitGroup
	for _, text := range []string{"first\n", "second\n", "third\n"} {
		call := fileCall(test, map[string]any{"path": "source.txt", "actions": []any{map[string]any{"op": "append", "content": text}}, "return": map[string]any{"type": "read"}})
		workers.Go(func() {
			result, operationError := NewFileActions(false).Run(operationContext, call)
			if operationError != nil {
				test.Error(operationError)
				return
			}
			if !strings.HasSuffix(result.Text(), text) {
				test.Errorf("return did not belong to its append: %s", result.Text())
			}
		})
	}
	workers.Wait()
	actual, operationError := os.ReadFile(path)
	testutil.RequireNoError(test, operationError)
	for _, text := range []string{"first\n", "second\n", "third\n"} {
		if strings.Count(string(actual), text) != 1 {
			test.Fatalf("lost or duplicated append: %q", actual)
		}
	}
}

func TestFileActionsCancellationReturnsOnlyTheFailure(test *testing.T) {
	operationContext, cancelOperation := context.WithCancel(harness.WithWorkspace(context.Background(), test.TempDir()))
	cancelOperation()
	result, operationError := NewFileActions(false).Run(operationContext, fileCall(test, map[string]any{"path": "missing.txt", "actions": []any{map[string]any{"op": "read"}}}))
	if !errors.Is(operationError, context.Canceled) || len(result.Content) != 0 {
		test.Fatalf("cancellation started recovery: %+v %v", result, operationError)
	}
}
