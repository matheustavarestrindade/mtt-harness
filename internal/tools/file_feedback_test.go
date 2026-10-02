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
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestFileEditFeedbackDescribesCommittedContent(test *testing.T) {
	for _, scenario := range []struct {
		name, original, expected string
		exists                   bool
		tool                     harness.Tool
		arguments                map[string]any
		feedback                 []string
	}{
		{"create", "", "one\ntwo\n", false, Write{}, map[string]any{"content": "one\ntwo\n"}, []string{"Created", "Lines: 0 -> 2", "Bytes: 0 -> 8", "--- /dev/null", "@@ -0,0 +1,2 @@", "+one\n+two\n"}},
		{"empty create", "", "", false, Write{}, map[string]any{"content": ""}, []string{"Created an empty file", "Lines: 0 -> 0"}},
		{"whole file", "old", "new\n", true, Write{}, map[string]any{"content": "new\n"}, []string{"Updated", "-old\n\\ No newline at end of file\n+new\n"}},
		{"range CRLF", "one\r\ntwo\r\nthree\r\n", "one\r\nnew\r\nthree\r\n", true, Write{}, map[string]any{"content": "new", "start_line": 2, "end_line": 2}, []string{"Replaced lines 2-2", "Lines: 3 -> 3", "-two\r\n+new\r\n"}},
		{"delete range", "one\ntwo\nthree\n", "one\nthree\n", true, Write{}, map[string]any{"content": "", "start_line": 2, "end_line": 2}, []string{"Lines: 3 -> 2", "-two\n"}},
		{"truncate", "old\n", "", true, Write{}, map[string]any{"content": ""}, []string{"Updated", "Lines: 1 -> 0", "@@ -1,1 +0,0 @@", "-old\n"}},
		{"write unchanged", "same", "same", true, Write{}, map[string]any{"content": "same"}, []string{"Unchanged", "No content changes"}},
		{"replace all", "cat cat\n", "dog dog\n", true, Replace{}, map[string]any{"old_text": "cat", "new_text": "dog", "mode": "all"}, []string{"Replaced 2 match(es) using mode \"all\"", "-cat cat\n+dog dog\n"}},
		{"replace unchanged", "cat\n", "cat\n", true, Replace{}, map[string]any{"old_text": "cat", "new_text": "cat"}, []string{"Unchanged", "Replaced 1 match(es)", "No content changes"}},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			workspace := test.TempDir()
			path := filepath.Join(workspace, "file.txt")
			if scenario.exists {
				testutil.RequireNoError(test, os.WriteFile(path, []byte(scenario.original), 0o600))
			}
			scenario.arguments["path"] = "file.txt"
			result, operationError := scenario.tool.Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, scenario.arguments))
			testutil.RequireNoError(test, operationError)
			if result.CallID != "edit-call" || result.Status != atom.StatusOK || result.Error != "" {
				test.Fatalf("unexpected result envelope: %+v", result)
			}
			for _, expected := range append(scenario.feedback, path) {
				if !strings.Contains(result.Text(), expected) {
					test.Fatalf("feedback lacks %q: %s", expected, result.Text())
				}
			}
			if scenario.exists && scenario.original == scenario.expected && strings.Contains(result.Text(), "@@") {
				test.Fatal("unchanged file reported a diff")
			}
			actual, operationError := os.ReadFile(path)
			testutil.RequireNoError(test, operationError)
			if string(actual) != scenario.expected {
				test.Fatalf("reported edit differs from committed bytes: %q", actual)
			}
		})
	}
}

func TestFileEditFailuresExplainRecoveryWithoutChangingTheFile(test *testing.T) {
	for _, scenario := range []struct {
		name      string
		tool      harness.Tool
		input     string
		fragments []string
	}{
		{"no match", Replace{}, `{"path":"file.txt","old_text":"missing","new_text":"new"}`, []string{"old_text was not found", "3 lines", "Use read", "case, whitespace, and LF/CRLF", "Do not retry"}},
		{"stale range", Write{}, `{"path":"file.txt","content":"new","end_line":8}`, []string{"end_line 8 exceeds", "3 lines", "read", "fresh read"}},
		{"invalid range", Replace{}, `{"path":"file.txt","old_text":"one","new_text":"new","start_line":0}`, []string{"start_line must be at least 1", "read", "1-based"}},
		{"missing content", Write{}, `{"path":"file.txt"}`, []string{"content are required", "content string"}},
		{"empty old text", Replace{}, `{"path":"file.txt","old_text":"","new_text":"new"}`, []string{"old_text must not be empty", "read", "exact text"}},
		{"missing new text", Replace{}, `{"path":"file.txt","old_text":"one"}`, []string{"new_text is required", "empty string to delete"}},
		{"invalid mode", Replace{}, `{"path":"file.txt","old_text":"one","new_text":"new","mode":"random"}`, []string{"mode must be first, last, or all", "omit mode"}},
		{"missing file", Replace{}, `{"path":"absent.txt","old_text":"one","new_text":"new"}`, []string{"no such file", "existing file"}},
		{"missing directory", Write{}, `{"path":"absent/file.txt","content":"new"}`, []string{"no such file", "parent directory"}},
		{"directory", Write{}, `{"path":".","content":"new"}`, []string{"regular file", "read"}},
		{"malformed JSON", Write{}, `{"path":"file.txt","content":`, []string{"unexpected end", "valid JSON"}},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			workspace := test.TempDir()
			path := filepath.Join(workspace, "file.txt")
			original := "one\ntwo\nthree\n"
			testutil.RequireNoError(test, os.WriteFile(path, []byte(original), 0o600))
			result, operationError := scenario.tool.Run(harness.WithWorkspace(context.Background(), workspace), atom.ToolCall{ID: "failed-edit", Input: []byte(scenario.input)})
			if operationError == nil || result.Status != atom.StatusError || result.CallID != "failed-edit" || result.Error != operationError.Error() {
				test.Fatalf("failure lost its envelope or cause: %+v, %v", result, operationError)
			}
			for _, expected := range append(scenario.fragments, "This call did not change the file") {
				if !strings.Contains(result.Text(), expected) {
					test.Fatalf("failure lacks %q: %s", expected, result.Text())
				}
			}
			actual, operationError := os.ReadFile(path)
			testutil.RequireNoError(test, operationError)
			if string(actual) != original {
				test.Fatal("failed edit changed the file")
			}
		})
	}
}

func TestFileEditFailurePreservesCancellationAndPermissionCauses(test *testing.T) {
	workspace := test.TempDir()
	operationContext, cancelOperation := context.WithCancel(harness.WithWorkspace(context.Background(), workspace))
	cancelOperation()
	result, operationError := (Write{}).Run(operationContext, fileCall(test, map[string]any{"path": "new.txt", "content": "new"}))
	if !errors.Is(operationError, context.Canceled) || !strings.Contains(result.Error, "cancelled") || result.Status != atom.StatusError {
		test.Fatalf("cancellation lost its cause or feedback: %+v, %v", result, operationError)
	}
	result, operationError = fileEditFailure(atom.ToolCall{ID: "permission"}, "write", "file.txt", &os.PathError{Op: "open", Path: "file.txt", Err: os.ErrPermission}, "")
	if !errors.Is(operationError, os.ErrPermission) || !strings.Contains(result.Error, "permission error") || !strings.Contains(result.Error, "read") {
		test.Fatalf("permission error lost its cause or recovery: %+v, %v", result, operationError)
	}
}

func TestConcurrentFileFeedbackUsesEachEditsOwnSnapshots(test *testing.T) {
	workspace := test.TempDir()
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("first\nsecond\n"), 0o600))
	operationContext := harness.WithWorkspace(context.Background(), workspace)
	var workers sync.WaitGroup
	for _, name := range []string{"first", "second"} {
		workers.Go(func() {
			result, operationError := (Replace{}).Run(operationContext, fileCall(test, map[string]any{"path": "file.txt", "old_text": name, "new_text": "new-" + name}))
			if operationError != nil {
				test.Error(operationError)
				return
			}
			if !strings.Contains(result.Text(), "-"+name+"\n+new-"+name+"\n") || strings.Count(result.Text(), "\n+new-") != 1 {
				test.Errorf("feedback includes another writer's change: %s", result.Text())
			}
		})
	}
	workers.Wait()
}
