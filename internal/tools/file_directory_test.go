package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestFileActionsListPagesUseSortedNamesAndDoNotFollowLinks(test *testing.T) {
	workspace := test.TempDir()
	var expected []string
	for index := 119; index >= 0; index-- {
		name := fmt.Sprintf("file-%03d.txt", index)
		testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, name), []byte("abc"), 0o600))
		expected = append(expected, name)
	}
	testutil.RequireNoError(test, os.Mkdir(filepath.Join(workspace, "folder"), 0o700))
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, ".hidden"), nil, 0o600))
	testutil.RequireNoError(test, os.Symlink(test.TempDir(), filepath.Join(workspace, "outside-link")))
	expected = append(expected, "folder", ".hidden", "outside-link")
	slices.Sort(expected)
	var actual []string
	cursor := ""
	for page := 0; page < 20; page++ {
		result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": ".", "actions": []any{map[string]any{"op": "list", "limit": 17, "cursor": cursor}}}))
		testutil.RequireNoError(test, operationError)
		text := result.Text()
		rows := strings.Split(text, "\n\n[")[0]
		for _, row := range strings.Split(rows, "\n") {
			if row == "" {
				continue
			}
			columns := strings.Split(row, "\t")
			if len(columns) != 3 {
				test.Fatalf("invalid listing row: %q", row)
			}
			name, operationError := strconv.Unquote(columns[0])
			testutil.RequireNoError(test, operationError)
			actual = append(actual, name)
			if name == "outside-link" && columns[1] != "symlink" {
				test.Fatal("listing followed a symlink target")
			}
			if name == "folder" && columns[1] != "directory" {
				test.Fatal("directory type missing")
			}
		}
		notice := strings.Index(text, "[Directory page limited")
		if notice < 0 {
			break
		}
		start := strings.IndexByte(text[notice:], '{') + notice
		end := strings.IndexByte(text[start:], '}') + start + 1
		var next struct {
			Cursor string `json:"cursor"`
		}
		testutil.RequireNoError(test, json.Unmarshal([]byte(text[start:end]), &next))
		if next.Cursor == cursor {
			test.Fatal("listing cursor made no progress")
		}
		cursor = next.Cursor
	}
	if !slices.Equal(actual, expected) {
		test.Fatalf("listing omitted, repeated or reordered names: got %d, want %d", len(actual), len(expected))
	}
}

func TestFileActionsDirectoryFailureDoesNotReturnAListing(test *testing.T) {
	workspace := test.TempDir()
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "existing.txt"), nil, 0o600))
	result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{
		"path": ".", "actions": []any{map[string]any{"op": "write", "content": "wrong target"}}, "return": map[string]any{"type": "summary"},
	}))
	if operationError == nil || !strings.Contains(result.Error, "regular file") || strings.Contains(result.Text(), "existing.txt") || len(result.Content) != 0 {
		test.Fatalf("directory failure returned other data: %+v", result)
	}
}
