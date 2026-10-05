package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestWriteReturnSelectsDiffContextOrUpdatedText(test *testing.T) {
	for _, scenario := range []struct {
		name               string
		selection          map[string]any
		includes, excludes []string
	}{
		{"default", nil, []string{" one\n", "-four\n+NEW\n", " seven\n"}, nil},
		{"diff", map[string]any{"type": "diff"}, []string{"@@ -4,1 +4,1 @@", "-four\n+NEW\n"}, []string{" three\n", " five\n"}},
		{"surrounding", map[string]any{"type": "surrounding", "size": 2}, []string{"@@ -2,5 +2,5 @@", " two\n three\n", " five\n six\n"}, []string{" one\n", " seven\n"}},
		{"zero context", map[string]any{"type": "surrounding", "size": 0}, []string{"@@ -4,1 +4,1 @@"}, []string{" three\n"}},
		{"lines", map[string]any{"type": "lines", "start_line": 3, "end_line": 5}, []string{"3: three\n4: NEW\n5: five\n"}, []string{"@@", "2: two", "6: six"}},
		{"file", map[string]any{"type": "file"}, []string{"1: one\n", "4: NEW\n", "7: seven\n"}, []string{"@@"}},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			workspace := test.TempDir()
			path := filepath.Join(workspace, "source.txt")
			testutil.RequireNoError(test, os.WriteFile(path, []byte("one\ntwo\nthree\nfour\nfive\nsix\nseven\n"), 0o600))
			arguments := map[string]any{"path": "source.txt", "content": "NEW", "start_line": 4, "end_line": 4}
			if scenario.selection != nil {
				arguments["return"] = scenario.selection
			}
			result, operationError := (Write{}).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, arguments))
			testutil.RequireNoError(test, operationError)
			for _, expected := range append(scenario.includes, "Updated", path, "Lines: 7 -> 7") {
				if !strings.Contains(result.Text(), expected) {
					test.Fatalf("missing %q in %s", expected, result.Text())
				}
			}
			for _, excluded := range scenario.excludes {
				if strings.Contains(result.Text(), excluded) {
					test.Fatalf("unrequested output %q in %s", excluded, result.Text())
				}
			}
			actual, operationError := os.ReadFile(path)
			testutil.RequireNoError(test, operationError)
			if string(actual) != "one\ntwo\nthree\nNEW\nfive\nsix\nseven\n" {
				test.Fatal("output mode changed the edit")
			}
		})
	}
}

func TestWritePreviewUsesPostEditLineNumbersAndPreservesNoOpOutput(test *testing.T) {
	workspace := test.TempDir()
	operationContext := harness.WithWorkspace(context.Background(), workspace)
	path := filepath.Join(workspace, "source.txt")
	testutil.RequireNoError(test, os.WriteFile(path, []byte("one\r\ntwo\r\nthree\r\nfour\r\n"), 0o600))
	result, operationError := (Write{}).Run(operationContext, fileCall(test, map[string]any{
		"path": "source.txt", "start_line": 2, "end_line": 3, "content": "new-1\r\nnew-2\r\nnew-3",
		"return": map[string]any{"type": "lines", "start_line": 3, "end_line": 99},
	}))
	testutil.RequireNoError(test, operationError)
	if !strings.Contains(result.Text(), "3: new-2\r\n4: new-3\r\n5: four\r\n") || strings.Contains(result.Text(), "2: new-1") {
		test.Fatalf("preview used pre-edit line numbers: %s", result.Text())
	}
	for _, content := range []string{"same", "same", ""} {
		result, operationError = (Write{}).Run(operationContext, fileCall(test, map[string]any{"path": "source.txt", "content": content, "return": map[string]any{"type": "file"}}))
		testutil.RequireNoError(test, operationError)
		if content == "same" && !strings.Contains(result.Text(), "1: same") {
			test.Fatalf("no-op lost requested preview: %s", result.Text())
		}
		if content == "" && !strings.Contains(result.Text(), "empty (0 lines)") {
			test.Fatalf("empty preview acquired a line: %s", result.Text())
		}
	}
}

func TestInvalidWriteReturnNeverChangesTheFile(test *testing.T) {
	for _, options := range []string{
		`null`, `[]`, `{}`, `{"type":"unknown"}`, `{"type":"file","size":1}`,
		`{"type":"surrounding","size":-1}`, `{"type":"surrounding","size":101}`,
		`{"type":"file","start_line":1}`, `{"type":"lines","start_line":0}`,
		`{"type":"lines","start_line":3,"end_line":2}`, `{"type":"file","unexpected":true}`,
		`{"type":"lines","start_line":2}`,
	} {
		test.Run(options, func(test *testing.T) {
			workspace := test.TempDir()
			path := filepath.Join(workspace, "source.txt")
			testutil.RequireNoError(test, os.WriteFile(path, []byte("original\nsecond\n"), 0o600))
			result, operationError := (Write{}).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": "source.txt", "content": "new", "return": json.RawMessage(options)}))
			if operationError == nil || result.Status != atom.StatusError || len(result.Content) != 0 || result.Text() != result.Error {
				test.Fatalf("bad preview did not fail safely: %v, %s", operationError, result.Text())
			}
			actual, operationError := os.ReadFile(path)
			testutil.RequireNoError(test, operationError)
			if string(actual) != "original\nsecond\n" {
				test.Fatal("invalid preview committed a write")
			}
		})
	}
}

func TestWriteFilePreviewCapDoesNotTruncateTheEdit(test *testing.T) {
	workspace := test.TempDir()
	operationContext := harness.WithWorkspace(context.Background(), workspace)
	content := strings.Repeat("row\n", 250)
	result, operationError := (Write{}).Run(operationContext, fileCall(test, map[string]any{"path": "source.txt", "content": content, "return": map[string]any{"type": "file"}}))
	testutil.RequireNoError(test, operationError)
	if !strings.Contains(result.Text(), "200: row\n") || strings.Contains(result.Text(), "201: row\n") {
		test.Fatalf("write preview was not capped: %s", result.Text())
	}
	_, continuation := splitPreviewContinuation(test, result.Text())
	continuation["path"] = "source.txt"
	rest, operationError := NewRead(false).Run(operationContext, fileCall(test, continuation))
	testutil.RequireNoError(test, operationError)
	if rest.Text() != strings.Repeat("row\n", 50) {
		test.Fatal("write preview cursor skipped or repeated lines")
	}
	actual, operationError := os.ReadFile(filepath.Join(workspace, "source.txt"))
	testutil.RequireNoError(test, operationError)
	if string(actual) != content {
		test.Fatal("preview limit truncated the file")
	}
}

func TestConcurrentWritesReturnTheirOwnUpdatedSnapshot(test *testing.T) {
	workspace := test.TempDir()
	operationContext := harness.WithWorkspace(context.Background(), workspace)
	var workers sync.WaitGroup
	for _, content := range []string{"first writer", "second writer"} {
		call := fileCall(test, map[string]any{"path": "source.txt", "content": content, "return": map[string]any{"type": "file"}})
		workers.Go(func() {
			result, operationError := (Write{}).Run(operationContext, call)
			if operationError != nil {
				test.Error(operationError)
				return
			}
			if !strings.HasSuffix(result.Text(), "1: "+content) {
				test.Errorf("write returned another snapshot: %s", result.Text())
			}
		})
	}
	workers.Wait()
}
