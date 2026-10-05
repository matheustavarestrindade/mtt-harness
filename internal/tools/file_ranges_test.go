package tools

import (
	"context"
	"encoding/json"
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
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func fileCall(test *testing.T, arguments map[string]any) atom.ToolCall {
	test.Helper()
	data, operationError := json.Marshal(arguments)
	testutil.RequireNoError(test, operationError)
	return atom.ToolCall{ID: "edit-call", Input: data}
}

func TestRangedReadPreservesTextAndAbsoluteLineNumbers(test *testing.T) {
	workspace := test.TempDir()
	operationContext := harness.WithWorkspace(context.Background(), workspace)
	text := "α\r\n\r\n" + strings.Repeat("x", 7_000) + "\nlast"
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "source.txt"), []byte(text), 0o600))
	for _, scenario := range []struct {
		name      string
		arguments map[string]any
		numbered  bool
		expected  string
	}{
		{"whole", map[string]any{"path": "source.txt"}, false, text},
		{"first", map[string]any{"path": "source.txt", "end_line": 1}, false, "α\r\n"},
		{"blank line", map[string]any{"path": "source.txt", "start_line": 2, "end_line": 2}, true, "2: \r\n"},
		{"long line", map[string]any{"path": "source.txt", "start_line": 3, "end_line": 3}, true, "3: " + strings.Repeat("x", 7_000) + "\n"},
		{"clamped end", map[string]any{"path": "source.txt", "start_line": 4, "end_line": 99}, true, "4: last"},
		{"open end", map[string]any{"path": "source.txt", "start_line": 4}, false, "last"},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			result, operationError := NewRead(scenario.numbered).Run(operationContext, fileCall(test, scenario.arguments))
			testutil.RequireNoError(test, operationError)
			if result.Text() != scenario.expected {
				test.Fatalf("read returned %d bytes, want %d; wrong selection or labels", len(result.Text()), len(scenario.expected))
			}
		})
	}
	for _, name := range []string{"empty.txt", "newline.txt"} {
		content := ""
		if name == "newline.txt" {
			content = "line\n"
		}
		testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, name), []byte(content), 0o600))
	}
	result, operationError := NewRead(true).Run(operationContext, fileCall(test, map[string]any{"path": "empty.txt"}))
	testutil.RequireNoError(test, operationError)
	if result.Text() != "" {
		test.Fatal("empty file acquired a phantom line")
	}
	if _, operationError := NewRead(true).Run(operationContext, fileCall(test, map[string]any{"path": "newline.txt", "start_line": 2})); operationError == nil {
		test.Fatal("trailing newline acquired a phantom line")
	}
}

func TestRangeWritesPreserveUntouchedBytesAndLineBoundaries(test *testing.T) {
	for _, scenario := range []struct {
		name, original, content, expected string
		bounds                            map[string]any
	}{
		{"middle", "one\ntwo\nthree\n", "new", "one\nnew\nthree\n", map[string]any{"start_line": 2, "end_line": 2}},
		{"crlf", "one\r\ntwo\r\nthree\r\n", "new", "one\r\nnew\r\nthree\r\n", map[string]any{"start_line": 2, "end_line": 2}},
		{"deletion", "one\ntwo\nthree", "", "one\nthree", map[string]any{"start_line": 2, "end_line": 2}},
		{"multiple new lines", "one\ntwo\nthree", "a\nb", "one\na\nb\nthree", map[string]any{"start_line": 2, "end_line": 2}},
		{"end without newline", "one\ntwo", "new", "one\nnew", map[string]any{"start_line": 2}},
		{"end with newline", "one\ntwo\n", "new", "one\nnew\n", map[string]any{"start_line": 2}},
		{"start omitted", "one\ntwo\nthree", "new\n", "new\nthree", map[string]any{"end_line": 2}},
		{"whole file verbatim", "one\ntwo\n", "new", "new", map[string]any{}},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			workspace := test.TempDir()
			path := filepath.Join(workspace, "source.txt")
			testutil.RequireNoError(test, os.WriteFile(path, []byte(scenario.original), 0o600))
			arguments := scenario.bounds
			arguments["path"] = "source.txt"
			arguments["content"] = scenario.content
			_, operationError := (Write{}).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, arguments))
			testutil.RequireNoError(test, operationError)
			actual, operationError := os.ReadFile(path)
			testutil.RequireNoError(test, operationError)
			if string(actual) != scenario.expected {
				test.Fatalf("file = %q, want %q", actual, scenario.expected)
			}
			information, operationError := os.Stat(path)
			testutil.RequireNoError(test, operationError)
			if information.Mode().Perm() != 0o600 {
				test.Fatalf("permissions changed: %v", information.Mode())
			}
		})
	}
}

func TestReplaceModesStayWithinSelectedLines(test *testing.T) {
	for _, scenario := range []struct{ mode, expected string }{
		{"first", "outside cat\nDOG cat\ncat\noutside cat\n"},
		{"last", "outside cat\ncat cat\nDOG\noutside cat\n"},
		{"all", "outside cat\nDOG DOG\nDOG\noutside cat\n"},
	} {
		test.Run(scenario.mode, func(test *testing.T) {
			workspace := test.TempDir()
			path := filepath.Join(workspace, "source.txt")
			testutil.RequireNoError(test, os.WriteFile(path, []byte("outside cat\ncat cat\ncat\noutside cat\n"), 0o600))
			_, operationError := (Replace{}).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": "source.txt", "old_text": "cat", "new_text": "DOG", "mode": scenario.mode, "start_line": 2, "end_line": 3}))
			testutil.RequireNoError(test, operationError)
			actual, operationError := os.ReadFile(path)
			testutil.RequireNoError(test, operationError)
			if string(actual) != scenario.expected {
				test.Fatalf("file = %q, want %q", actual, scenario.expected)
			}
		})
	}
}

func TestReplaceIsLiteralMultilineAndNonRecursive(test *testing.T) {
	for _, scenario := range []struct{ name, original, oldText, newText, mode, expected string }{
		{"default first", "aaa", "aa", "x", "", "xa"},
		{"last overlapping", "aaa", "aa", "x", "last", "ax"},
		{"all non-overlapping", "aaaaa", "aa", "x", "all", "xxa"},
		{"inserted text not searched", "a a", "a", "aa", "all", "aa aa"},
		{"literal syntax", "$1.* $1.*", "$1.*", `${value}\path`, "all", `${value}\path ${value}\path`},
		{"multiline", "α\r\nb\r\nc", "α\r\nb", "new", "first", "new\r\nc"},
		{"deletion", "one two one", "one", "", "all", " two "},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			workspace := test.TempDir()
			path := filepath.Join(workspace, "source.txt")
			testutil.RequireNoError(test, os.WriteFile(path, []byte(scenario.original), 0o600))
			arguments := map[string]any{"path": "source.txt", "old_text": scenario.oldText, "new_text": scenario.newText}
			if scenario.mode != "" {
				arguments["mode"] = scenario.mode
			}
			_, operationError := (Replace{}).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, arguments))
			testutil.RequireNoError(test, operationError)
			actual, operationError := os.ReadFile(path)
			testutil.RequireNoError(test, operationError)
			if string(actual) != scenario.expected {
				test.Fatalf("file = %q, want %q", actual, scenario.expected)
			}
		})
	}
}

func TestInvalidFileEditsLeaveOriginalUnchanged(test *testing.T) {
	workspace := test.TempDir()
	path := filepath.Join(workspace, "source.txt")
	original := "one\ntwo\nthree\n"
	testutil.RequireNoError(test, os.WriteFile(path, []byte(original), 0o600))
	operationContext := harness.WithWorkspace(context.Background(), workspace)
	for _, arguments := range []map[string]any{
		{"content": "bad", "start_line": 0}, {"content": "bad", "start_line": 3, "end_line": 2}, {"content": "bad", "end_line": 5},
		{"old_text": "", "new_text": "bad"}, {"old_text": "absent", "new_text": "bad"}, {"old_text": "ONE", "new_text": "bad"},
		{"old_text": "one", "new_text": "bad", "mode": "random"}, {"old_text": "one", "new_text": "bad", "mode": ""},
		{"old_text": "one", "new_text": "bad", "start_line": 2}, {"old_text": "two\nthree", "new_text": "bad", "start_line": 2, "end_line": 2},
	} {
		arguments["path"] = "source.txt"
		var tool harness.Tool = Replace{}
		if _, found := arguments["content"]; found {
			tool = Write{}
		}
		if _, operationError := tool.Run(operationContext, fileCall(test, arguments)); operationError == nil {
			test.Fatalf("invalid edit accepted: %+v", arguments)
		}
		actual, operationError := os.ReadFile(path)
		testutil.RequireNoError(test, operationError)
		if string(actual) != original {
			test.Fatalf("invalid edit changed file: %q", actual)
		}
	}
	cancelled, cancel := context.WithCancel(operationContext)
	cancel()
	if _, operationError := (Write{}).Run(cancelled, fileCall(test, map[string]any{"path": "source.txt", "content": "bad", "start_line": 1})); operationError == nil {
		test.Fatal("cancelled edit succeeded")
	}
	if _, operationError := (Write{}).Run(operationContext, fileCall(test, map[string]any{"path": "missing.txt", "content": "bad", "start_line": 1})); operationError == nil {
		test.Fatal("range write created a missing file")
	}
	if _, operationError := os.Stat(filepath.Join(workspace, "missing.txt")); !os.IsNotExist(operationError) {
		test.Fatalf("invalid write created a file: %v", operationError)
	}
}

func TestConcurrentLiteralEditsDoNotLoseUpdates(test *testing.T) {
	workspace := test.TempDir()
	path := filepath.Join(workspace, "source.txt")
	var original, expected strings.Builder
	for index := 0; index < 20; index++ {
		fmt.Fprintf(&original, "token-%02d\n", index)
		fmt.Fprintf(&expected, "value-%02d\n", index)
	}
	testutil.RequireNoError(test, os.WriteFile(path, []byte(original.String()), 0o600))
	operationContext := harness.WithWorkspace(context.Background(), workspace)
	var workers sync.WaitGroup
	results := make(chan error, 20)
	for index := 0; index < 20; index++ {
		call := fileCall(test, map[string]any{"path": "source.txt", "old_text": fmt.Sprintf("token-%02d", index), "new_text": fmt.Sprintf("value-%02d", index)})
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, operationError := (Replace{}).Run(operationContext, call)
			results <- operationError
		}()
	}
	workers.Wait()
	close(results)
	for operationError := range results {
		testutil.RequireNoError(test, operationError)
	}
	actual, operationError := os.ReadFile(path)
	testutil.RequireNoError(test, operationError)
	if string(actual) != expected.String() {
		test.Fatalf("concurrent edits were lost: %q", actual)
	}
}

func TestFileEditGateHonorsCancellationWithoutDroppingItsOwner(test *testing.T) {
	workspace := test.TempDir()
	path := filepath.Join(workspace, "source.txt")
	testutil.RequireNoError(test, os.WriteFile(path, []byte("old\n"), 0o600))
	release, operationError := acquireFileEdit(context.Background(), path)
	testutil.RequireNoError(test, operationError)
	release = sync.OnceFunc(release)
	defer release()
	operationContext, cancel := context.WithTimeout(harness.WithWorkspace(context.Background(), workspace), 20*time.Millisecond)
	defer cancel()
	_, operationError = (Replace{}).Run(operationContext, fileCall(test, map[string]any{"path": "source.txt", "old_text": "old", "new_text": "bad"}))
	if !errors.Is(operationError, context.DeadlineExceeded) {
		test.Fatalf("waiting edit ignored cancellation: %v", operationError)
	}
	actual, operationError := os.ReadFile(path)
	testutil.RequireNoError(test, operationError)
	if string(actual) != "old\n" {
		test.Fatal("cancelled waiter changed the file")
	}
	release()
	_, operationError = (Replace{}).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": "source.txt", "old_text": "old", "new_text": "new"}))
	testutil.RequireNoError(test, operationError)
}

func TestAtomicEditsResolveSymlinksAndPreserveOtherHardLinks(test *testing.T) {
	workspace := test.TempDir()
	path := filepath.Join(workspace, "source.txt")
	testutil.RequireNoError(test, os.WriteFile(path, []byte("old\n"), 0o600))
	testutil.RequireNoError(test, os.Link(path, filepath.Join(workspace, "hard.txt")))
	testutil.RequireNoError(test, os.Symlink(path, filepath.Join(workspace, "symbolic.txt")))
	_, operationError := (Replace{}).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": "symbolic.txt", "old_text": "old", "new_text": "new"}))
	testutil.RequireNoError(test, operationError)
	for name, expected := range map[string]string{"source.txt": "new\n", "symbolic.txt": "new\n", "hard.txt": "old\n"} {
		actual, operationError := os.ReadFile(filepath.Join(workspace, name))
		testutil.RequireNoError(test, operationError)
		if string(actual) != expected {
			test.Fatalf("%s = %q", name, actual)
		}
	}
	information, operationError := os.Lstat(filepath.Join(workspace, "symbolic.txt"))
	testutil.RequireNoError(test, operationError)
	if information.Mode()&os.ModeSymlink == 0 {
		test.Fatal("edit replaced the symbolic link itself")
	}
}
