package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestFileActionsEmitOnlyTheSelectedReturn(test *testing.T) {
	for _, outputType := range []string{"read", "diff", "summary"} {
		test.Run(outputType, func(test *testing.T) {
			workspace := test.TempDir()
			testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "file.txt"), []byte("before\n"), 0o600))
			result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{
				"path": "file.txt", "actions": []any{
					map[string]any{"op": "read"},
					map[string]any{"op": "write", "content": "after\n"},
					map[string]any{"op": "read"},
				}, "return": map[string]any{"type": outputType},
			}))
			testutil.RequireNoError(test, operationError)
			switch outputType {
			case "read":
				if result.Text() != "after\n" {
					test.Fatalf("extra read output: %q", result.Text())
				}
			case "summary":
				if result.Text() != "Updated" {
					test.Fatalf("extra summary output: %q", result.Text())
				}
			case "diff":
				if !strings.HasPrefix(result.Text(), "--- ") || !strings.Contains(result.Text(), "-before\n+after\n") || strings.Contains(result.Text(), "Actions completed") || strings.Contains(result.Text(), "Return diff") {
					test.Fatalf("extra diff output: %q", result.Text())
				}
			}
		})
	}
}

func TestFinalPreviewOwnsItsBudgetAndCanBeEmpty(test *testing.T) {
	workspace := test.TempDir()
	content := strings.Repeat("row\n", 250)
	operationContext := harness.WithWorkspace(context.Background(), workspace)
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "file.txt"), []byte(content), 0o600))
	result, operationError := NewFileActions(false).Run(operationContext, fileCall(test, map[string]any{
		"path": "file.txt", "actions": []any{map[string]any{"op": "read"}, map[string]any{"op": "read"}}, "return": map[string]any{"type": "read"},
	}))
	testutil.RequireNoError(test, operationError)
	if strings.Count(result.Text(), "row\n") != 200 || strings.Count(result.Text(), "Continuation") != 1 || !strings.Contains(result.Text(), `"start_line":201`) {
		test.Fatal("intermediate reads consumed or duplicated the final preview")
	}
	result, operationError = NewFileActions(false).Run(operationContext, fileCall(test, map[string]any{
		"path": "file.txt", "actions": []any{map[string]any{"op": "write", "content": ""}}, "return": map[string]any{"type": "read"},
	}))
	testutil.RequireNoError(test, operationError)
	if result.Text() != "" {
		test.Fatalf("empty file acquired synthetic output: %q", result.Text())
	}
}
