package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestFileActionsRejectIncompatibleFieldsAndUnsafeRecovery(test *testing.T) {
	for _, encoded := range []string{
		`{"path":"file.txt","actions":[]}`,
		`{"path":"file.txt","actions":[{"op":"read","content":"ignored"}]}`,
		`{"path":"file.txt","actions":[{"op":"append"}],"return":{"type":"summary"}}`,
		`{"path":"file.txt","actions":[{"op":"read"}],"return":{"type":"diff"}}`,
		`{"path":"file.txt","actions":[{"op":"write","content":"changed"}],"return":{"type":"summary"},"on_error":{"return":{"type":"write","content":"bad"}}}`,
		`{"path":"file.txt","actions":[{"op":"write","content":"changed"}],"return":{"type":"read","context_lines":3}}`,
		`{"path":"file.txt","actions":[{"op":"list","limit":0}]}`,
		`{"path":"file.txt","actions":[{"op":"list","cursor":"not a cursor"}]}`,
		`{"path":"file.txt","actions":[{"op":"list"},{"op":"write","content":"bad"}],"return":{"type":"summary"}}`,
		`{"path":"file.txt","actions":[{"op":"read","start_byte":null}]}`,
		`{"path":"file.txt","actions":[{"op":"read"}],"return":{"type":"list"}}`,
		`{"path":"file.txt","actions":[{"op":"write","content":"bad"}],"return":{"type":"diff","context_lines":101}}`,
	} {
		workspace := test.TempDir()
		path := filepath.Join(workspace, "file.txt")
		testutil.RequireNoError(test, os.WriteFile(path, []byte("original"), 0o600))
		result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), atom.ToolCall{ID: "invalid", Input: []byte(encoded)})
		if operationError == nil || result.Status != atom.StatusError {
			test.Fatalf("invalid input accepted: %s", encoded)
		}
		actual, operationError := os.ReadFile(path)
		testutil.RequireNoError(test, operationError)
		if string(actual) != "original" {
			test.Fatalf("invalid input mutated a file: %s", encoded)
		}
	}
}

func TestFileActionsLongLineCursorsRoundTripUTF8(test *testing.T) {
	workspace := test.TempDir()
	original := strings.Repeat("日本😀", 9000) + "\nend"
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "file.txt"), []byte(original), 0o600))
	action := map[string]any{"op": "read"}
	var reconstructed strings.Builder
	for page := 0; page < 30; page++ {
		result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": "file.txt", "actions": []any{action}}))
		testutil.RequireNoError(test, operationError)
		text := result.Text()
		notice := strings.Index(text, "\n\n[Shared file_actions preview budget")
		if notice < 0 {
			reconstructed.WriteString(text)
			break
		}
		preview := text[:notice]
		if len(preview) > filePreviewMaxBytes || !utf8.ValidString(preview) {
			test.Fatal("invalid UTF-8 or oversized preview")
		}
		reconstructed.WriteString(preview)
		start := strings.IndexByte(text[notice:], '{') + notice
		end := strings.IndexByte(text[start:], '}') + start + 1
		testutil.RequireNoError(test, json.Unmarshal([]byte(text[start:end]), &action))
	}
	if reconstructed.String() != original {
		test.Fatal("unified read continuation lost or repeated file bytes")
	}
}

func TestFileActionsDiffPreservesLiteralRecoveryInstructionsInSource(test *testing.T) {
	workspace := test.TempDir()
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("old\n"), 0o600))
	content := "Use read to inspect\nUse read from line 2\n"
	result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": "file.txt", "actions": []any{map[string]any{"op": "write", "content": content}}, "return": map[string]any{"type": "diff", "context_lines": 0}}))
	testutil.RequireNoError(test, operationError)
	if !strings.Contains(result.Text(), "+Use read to inspect\n+Use read from line 2\n") {
		test.Fatalf("diff rewrote literal file data: %s", result.Text())
	}
}
