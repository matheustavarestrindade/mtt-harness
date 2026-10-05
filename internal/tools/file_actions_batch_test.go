package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	inputSchemaValidation "github.com/matheustavarestrindade/mtt-harness/internal/molecule/schema"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestMultipleFilesReuseAnOrderedActionChain(test *testing.T) {
	workspace := test.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, name), []byte("old\n"), 0o600))
	}
	fileTool := NewFileActions(false)
	call := fileCall(test, map[string]any{
		"paths":   []string{"a.txt", "b.txt"},
		"actions": []any{map[string]any{"op": "replace", "old_text": "old", "new_text": "new"}, map[string]any{"op": "append", "content": "tail\n"}, map[string]any{"op": "prepend", "content": "head\n"}},
		"return":  map[string]any{"type": "read"},
	})
	testutil.RequireNoError(test, inputSchemaValidation.Validate(fileTool.InputSchema().JSON, call.Input))
	result, operationError := fileTool.Run(harness.WithWorkspace(context.Background(), workspace), call)
	testutil.RequireNoError(test, operationError)
	for _, name := range []string{"a.txt", "b.txt"} {
		content, operationError := os.ReadFile(filepath.Join(workspace, name))
		testutil.RequireNoError(test, operationError)
		if string(content) != "head\nnew\ntail\n" {
			test.Fatalf("incorrect shared action result for %s: %q", name, content)
		}
	}
	if result.Text() != "\"a.txt\"\nhead\nnew\ntail\n\n\"b.txt\"\nhead\nnew\ntail\n" {
		test.Fatalf("unclear or repeated output: %q", result.Text())
	}
	result, operationError = fileTool.Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"paths": []string{"a.txt", "b.txt"}, "actions": []any{map[string]any{"op": "delete"}}, "return": map[string]any{"type": "summary"}}))
	testutil.RequireNoError(test, operationError)
	if result.Text() != "Deleted 2" {
		test.Fatalf("verbose batch summary: %s", result.Text())
	}
	entries, operationError := os.ReadDir(workspace)
	testutil.RequireNoError(test, operationError)
	if len(entries) != 0 {
		test.Fatal("batch deletion left files or temporary data")
	}
}

func TestBatchPreparationFailureChangesNoTargets(test *testing.T) {
	for _, operation := range []string{"replace", "append", "delete"} {
		test.Run(operation, func(test *testing.T) {
			workspace := test.TempDir()
			path := filepath.Join(workspace, "first.txt")
			testutil.RequireNoError(test, os.WriteFile(path, []byte("before"), 0o600))
			action := map[string]any{"op": operation}
			if operation == "replace" {
				action["old_text"] = "before"
				action["new_text"] = "after"
			}
			if operation == "append" {
				action["content"] = "after"
			}
			result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"paths": []string{"first.txt", "absent.txt"}, "actions": []any{action}, "return": map[string]any{"type": "summary"}}))
			if !errors.Is(operationError, os.ErrNotExist) || !strings.Contains(result.Error, "Target index: 2") || strings.Contains(result.Error, "Committed target indexes") || len(result.Content) != 0 {
				test.Fatalf("incorrect failure: %+v %v", result, operationError)
			}
			content, operationError := os.ReadFile(path)
			testutil.RequireNoError(test, operationError)
			if string(content) != "before" {
				test.Fatal("earlier target changed before preparation completed")
			}
			entries, operationError := os.ReadDir(workspace)
			testutil.RequireNoError(test, operationError)
			if len(entries) != 1 {
				test.Fatal("preparation failure leaked temporary files")
			}
		})
	}
}

func TestBatchRejectsAliasesAndOverlappingMutationPaths(test *testing.T) {
	workspace := test.TempDir()
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "file"), []byte("before"), 0o600))
	testutil.RequireNoError(test, os.Symlink("file", filepath.Join(workspace, "alias")))
	for _, paths := range [][]string{{"file", "./file"}, {"file", "alias"}, {"file", "file/child"}} {
		_, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"paths": paths, "actions": []any{map[string]any{"op": "append", "content": "after"}}, "return": map[string]any{"type": "summary"}}))
		if operationError == nil {
			test.Fatalf("invalid target set accepted: %v", paths)
		}
	}
	content, operationError := os.ReadFile(filepath.Join(workspace, "file"))
	testutil.RequireNoError(test, operationError)
	if string(content) != "before" {
		test.Fatal("invalid target set changed a file")
	}
}

func TestBatchOutputPreparationFailsBeforeAnyWrite(test *testing.T) {
	workspace := test.TempDir()
	for name, text := range map[string]string{"long": "one\ntwo\nthree\n", "short": "one\n"} {
		testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, name), []byte(text), 0o600))
	}
	result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{
		"paths": []string{"long", "short"}, "actions": []any{map[string]any{"op": "append", "content": "new\n"}}, "return": map[string]any{"type": "read", "start_line": 3},
	}))
	if operationError == nil || !strings.Contains(result.Error, "Target index: 2") {
		test.Fatalf("invalid later preview accepted: %+v", result)
	}
	for name, expected := range map[string]string{"long": "one\ntwo\nthree\n", "short": "one\n"} {
		content, operationError := os.ReadFile(filepath.Join(workspace, name))
		testutil.RequireNoError(test, operationError)
		if string(content) != expected {
			test.Fatal("a preview error committed earlier targets")
		}
	}
	entries, operationError := os.ReadDir(workspace)
	testutil.RequireNoError(test, operationError)
	if len(entries) != 2 {
		test.Fatal("preview failure leaked prepared replacement data")
	}
}

func TestBatchFileSystemCommitFailureKeepsActualCommitIndexes(test *testing.T) {
	workspace := test.TempDir()
	plans := make([]fileActionPlan, 3)
	for index := range plans {
		path := filepath.Join(workspace, fmt.Sprint(index))
		testutil.RequireNoError(test, os.WriteFile(path, []byte("before"), 0o600))
		information, operationError := os.Stat(path)
		testutil.RequireNoError(test, operationError)
		replacement, operationError := prepareFileReplacement(context.Background(), path, information, []byte("after"))
		testutil.RequireNoError(test, operationError)
		defer replacement.discard()
		plans[index] = fileActionPlan{path: path, commit: replacement.commit}
	}
	// Replace a destination after preparation to force a real rename failure.
	testutil.RequireNoError(test, os.Remove(plans[1].path))
	testutil.RequireNoError(test, os.Mkdir(plans[1].path, 0o700))
	failed, committed, operationError := commitFileActionPlans(context.Background(), plans)
	if operationError == nil || failed != 1 || len(committed) != 1 || committed[0] != 1 {
		test.Fatalf("incorrect filesystem commit outcome: %d %v %v", failed, committed, operationError)
	}
	first, operationError := os.ReadFile(plans[0].path)
	testutil.RequireNoError(test, operationError)
	third, operationError := os.ReadFile(plans[2].path)
	testutil.RequireNoError(test, operationError)
	if string(first) != "after" || string(third) != "before" {
		test.Fatal("commit report disagrees with the actual files")
	}
}

func TestBatchOutputHasOneBudgetAndNoCrossTargetDuplicates(test *testing.T) {
	workspace := test.TempDir()
	operationContext := harness.WithWorkspace(context.Background(), workspace)
	for _, name := range []string{"a", "b", "c"} {
		testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, name), []byte(strings.Repeat(name+"-row\n", 250)), 0o600))
	}
	result, operationError := NewFileActions(false).Run(operationContext, fileCall(test, map[string]any{"paths": []string{"a", "b", "c"}, "actions": []any{map[string]any{"op": "read"}}}))
	testutil.RequireNoError(test, operationError)
	if strings.Count(result.Text(), "a-row\n") != 199 || strings.Contains(result.Text(), "b-row") || !strings.Contains(result.Text(), "Next target index: 2") {
		test.Fatalf("unbounded multi-file output: %q", result.Text())
	}
	result, operationError = NewFileActions(false).Run(operationContext, fileCall(test, map[string]any{"paths": []string{"a", "b"}, "actions": []any{map[string]any{"op": "read", "end_line": 1}, map[string]any{"op": "read", "start_line": 2, "end_line": 2}}}))
	testutil.RequireNoError(test, operationError)
	if strings.Count(result.Text(), "a-row") != 2 || strings.Count(result.Text(), "b-row") != 2 {
		test.Fatal("inspection text leaked across targets")
	}
}

func TestBatchCommitFailureReportsPriorCommitsAndStops(test *testing.T) {
	commitError := errors.New("injected commit failure")
	var lastRan bool
	plans := []fileActionPlan{
		{commit: func(context.Context) error { return nil }},
		{commit: func(context.Context) error { return commitError }},
		{commit: func(context.Context) error { lastRan = true; return nil }},
	}
	index, committed, operationError := commitFileActionPlans(context.Background(), plans)
	if index != 1 || len(committed) != 1 || committed[0] != 1 || !errors.Is(operationError, commitError) || lastRan {
		test.Fatal("commit failure lost partial outcome or continued")
	}
	result, failure := failFileActionTarget(atom.ToolCall{}, "second.txt", operationError, nil, index, true, committed)
	if !errors.Is(failure, commitError) || !strings.Contains(result.Error, "Committed target indexes: [1]") || strings.Contains(result.Error, "retry") {
		test.Fatalf("partial failure is not factual: %s", result.Error)
	}
}

func TestConcurrentBatchesUseCanonicalGateOrder(test *testing.T) {
	workspace := test.TempDir()
	for _, name := range []string{"a", "b"} {
		testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, name), nil, 0o600))
	}
	operationContext, cancel := context.WithTimeout(harness.WithWorkspace(context.Background(), workspace), 5*time.Second)
	defer cancel()
	var workers sync.WaitGroup
	for index := range 12 {
		paths := []string{"a", "b"}
		if index%2 == 1 {
			paths = []string{"b", "a"}
		}
		call := fileCall(test, map[string]any{"paths": paths, "actions": []any{map[string]any{"op": "append", "content": fmt.Sprintf("%d\n", index)}}, "return": map[string]any{"type": "summary"}})
		workers.Go(func() {
			_, operationError := NewFileActions(false).Run(operationContext, call)
			if operationError != nil {
				test.Error(operationError)
			}
		})
	}
	workers.Wait()
	first, operationError := os.ReadFile(filepath.Join(workspace, "a"))
	testutil.RequireNoError(test, operationError)
	second, operationError := os.ReadFile(filepath.Join(workspace, "b"))
	testutil.RequireNoError(test, operationError)
	if string(first) != string(second) || strings.Count(string(first), "\n") != 12 {
		test.Fatal("overlapping batches lost or interleaved actions")
	}
}
