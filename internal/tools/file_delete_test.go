package tools

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	inputSchemaValidation "github.com/matheustavarestrindade/mtt-harness/internal/molecule/schema"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestDeleteRemovesOnlyTheRequestedEntry(test *testing.T) {
	workspace := test.TempDir()
	outside := filepath.Join(test.TempDir(), "outside.txt")
	testutil.RequireNoError(test, os.WriteFile(outside, []byte("keep"), 0o600))
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "file"), []byte("data"), 0o600))
	testutil.RequireNoError(test, os.Link(filepath.Join(workspace, "file"), filepath.Join(workspace, "hard-link")))
	testutil.RequireNoError(test, os.Mkdir(filepath.Join(workspace, "empty"), 0o700))
	testutil.RequireNoError(test, os.Symlink(outside, filepath.Join(workspace, "link")))
	testutil.RequireNoError(test, os.Symlink(filepath.Join(workspace, "absent"), filepath.Join(workspace, "dangling")))
	for _, path := range []string{"file", "empty", "link", "dangling"} {
		call := fileCall(test, map[string]any{"path": path, "actions": []any{map[string]any{"op": "delete"}}, "return": map[string]any{"type": "summary"}})
		fileTool := NewFileActions(false)
		testutil.RequireNoError(test, inputSchemaValidation.Validate(fileTool.InputSchema().JSON, call.Input))
		result, operationError := fileTool.Run(harness.WithWorkspace(context.Background(), workspace), call)
		testutil.RequireNoError(test, operationError)
		if result.Text() != "Deleted" {
			test.Fatalf("delete output is not compact: %q", result.Text())
		}
		if _, operationError := os.Lstat(filepath.Join(workspace, path)); !os.IsNotExist(operationError) {
			test.Fatalf("entry remains: %s", path)
		}
	}
	for path, wanted := range map[string]string{outside: "keep", filepath.Join(workspace, "hard-link"): "data"} {
		data, operationError := os.ReadFile(path)
		testutil.RequireNoError(test, operationError)
		if string(data) != wanted {
			test.Fatal("deletion changed another link or target")
		}
	}
}

func TestDeleteFailuresLeaveContentsAndDirectoriesIntact(test *testing.T) {
	workspace := test.TempDir()
	file := filepath.Join(workspace, "file")
	testutil.RequireNoError(test, os.WriteFile(file, []byte("original"), 0o600))
	testutil.RequireNoError(test, os.Mkdir(filepath.Join(workspace, "folder"), 0o700))
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "folder", "child"), []byte("keep"), 0o600))
	for _, scenario := range []map[string]any{
		{"path": ".", "actions": []any{map[string]any{"op": "delete"}}, "return": map[string]any{"type": "summary"}},
		{"path": "folder", "actions": []any{map[string]any{"op": "delete"}}, "return": map[string]any{"type": "summary"}},
		{"path": "file", "actions": []any{map[string]any{"op": "delete"}}, "return": map[string]any{"type": "read"}},
		{"path": "file", "actions": []any{map[string]any{"op": "delete"}, map[string]any{"op": "append", "content": "bad"}}, "return": map[string]any{"type": "summary"}},
		{"path": "file", "actions": []any{map[string]any{"op": "delete", "recursive": true}}, "return": map[string]any{"type": "summary"}},
	} {
		if _, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, scenario)); operationError == nil {
			test.Fatalf("invalid delete succeeded: %v", scenario)
		}
		data, operationError := os.ReadFile(file)
		testutil.RequireNoError(test, operationError)
		if string(data) != "original" {
			test.Fatal("failed deletion changed original content")
		}
		if _, operationError := os.Stat(filepath.Join(workspace, "folder", "child")); operationError != nil {
			test.Fatal("directory deletion became recursive")
		}
	}
	_, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": "absent", "actions": []any{map[string]any{"op": "delete"}}, "return": map[string]any{"type": "summary"}}))
	if !errors.Is(operationError, os.ErrNotExist) {
		test.Fatalf("missing entry lost its cause: %v", operationError)
	}
}

func TestDeleteCanBeStagedAndRecreated(test *testing.T) {
	workspace := test.TempDir()
	path := filepath.Join(workspace, "file")
	testutil.RequireNoError(test, os.WriteFile(path, []byte("before"), 0o640))
	result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{
		"path": "file", "actions": []any{map[string]any{"op": "delete"}, map[string]any{"op": "write", "content": "after"}}, "return": map[string]any{"type": "read"},
	}))
	testutil.RequireNoError(test, operationError)
	data, operationError := os.ReadFile(path)
	testutil.RequireNoError(test, operationError)
	if result.Text() != "after" || string(data) != "after" {
		test.Fatal("ordered deletion/recreation has incorrect final content")
	}
}

func TestDeleteProtectsAWorkspaceConfiguredThroughALink(test *testing.T) {
	root := test.TempDir()
	workspace := filepath.Join(test.TempDir(), "workspace-link")
	testutil.RequireNoError(test, os.Symlink(root, workspace))
	_, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": ".", "actions": []any{map[string]any{"op": "delete"}}, "return": map[string]any{"type": "summary"}}))
	if operationError == nil {
		test.Fatal("workspace entry was deleted")
	}
	if _, operationError := os.Lstat(workspace); operationError != nil {
		test.Fatal("workspace link changed")
	}
	if _, operationError := os.Stat(root); operationError != nil {
		test.Fatal("workspace target changed")
	}
}

func TestDeletedFileDiffIsAcceptedByGit(test *testing.T) {
	gitPath, operationError := exec.LookPath("git")
	if operationError != nil {
		test.Skip("git unavailable")
	}
	workspace := test.TempDir()
	path := filepath.Join(workspace, "file with spaces")
	original := []byte("first\r\nlast")
	testutil.RequireNoError(test, os.WriteFile(path, original, 0o600))
	result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": path, "actions": []any{map[string]any{"op": "delete"}}, "return": map[string]any{"type": "diff", "context_lines": 0}}))
	testutil.RequireNoError(test, operationError)
	if !strings.Contains(result.Text(), "+++ /dev/null\n") || !strings.HasPrefix(result.Text(), "--- ") {
		test.Fatalf("delete did not return only its diff: %s", result.Text())
	}
	testutil.RequireNoError(test, os.WriteFile(path, original, 0o600))
	command := exec.Command(gitPath, "apply", "--unsafe-paths", "--unidiff-zero", "-p0", "-")
	command.Dir = workspace
	command.Stdin = strings.NewReader(result.Text())
	output, operationError := command.CombinedOutput()
	if operationError != nil {
		test.Fatalf("git rejected deletion diff: %v\n%s\n%s", operationError, output, result.Text())
	}
	if _, operationError := os.Stat(path); !os.IsNotExist(operationError) {
		test.Fatal("deletion diff did not remove the file")
	}
}
