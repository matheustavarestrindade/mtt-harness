package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

// Git applies the reported diff to independent before bytes. This verifies hunk
// coordinates and line endings against an external parser, not our formatter.
func TestFileEditDiffAppliesToOriginalBytes(test *testing.T) {
	gitPath, operationError := exec.LookPath("git")
	if operationError != nil {
		test.Skip("git is unavailable for independent diff verification")
	}
	for _, scenario := range []struct {
		name, original, updated string
		create                  bool
	}{
		{"creation", "", "one\ntwo\n", true},
		{"empty original", "", "new\n", false},
		{"deletion", "one\ntwo\n", "", false},
		{"middle", "one\ntwo\nthree\n", "one\nnew\nthree\n", false},
		{"missing final newline", "old", "new", false},
		{"add final newline", "same", "same\n", false},
		{"remove final newline", "same\n", "same", false},
		{"CRLF", "one\r\ntwo\r\nthree\r\n", "one\r\nnew\r\nthree\r\n", false},
		{"Unicode", "ação\n東京\n", "ação\n大阪\n", false},
		{"append after LF", "one\n", "one\ntwo\n", false},
		{"multiple hunks", "first\n" + strings.Repeat("same\n", 20) + "last\n", "FIRST\n" + strings.Repeat("same\n", 20) + "LAST\n", false},
		{"large unchanged prefix", strings.Repeat("same\n", 10000) + "old\ntail\n", strings.Repeat("same\n", 10000) + "new\ntail\n", false},
	} {
		for _, contextLines := range []int{0, 1, 3, 10} {
			test.Run(scenario.name+"/context="+strconv.Itoa(contextLines), func(test *testing.T) {
				workspace := test.TempDir()
				path := filepath.Join(workspace, "file with spaces.txt")
				if !scenario.create {
					testutil.RequireNoError(test, os.WriteFile(path, []byte(scenario.original), 0o600))
				}
				preview := renderFileEditDiff(path, []byte(scenario.original), []byte(scenario.updated), !scenario.create, contextLines)
				command := exec.Command(gitPath, "apply", "--unsafe-paths", "--unidiff-zero", "--whitespace=nowarn", "-p0", "-")
				command.Dir = workspace
				command.Stdin = strings.NewReader(preview)
				output, operationError := command.CombinedOutput()
				if operationError != nil {
					test.Fatalf("git rejected the preview: %v\n%s\n%s", operationError, output, preview)
				}
				actual, operationError := os.ReadFile(path)
				testutil.RequireNoError(test, operationError)
				if string(actual) != scenario.updated {
					test.Fatalf("diff applied to different bytes: %q, want %q", actual, scenario.updated)
				}
			})
		}
	}
}

func TestFileEditPreviewLimitsNeverLimitTheWrite(test *testing.T) {
	for _, scenario := range []struct{ name, content, notice string }{
		{"line limit", strings.Repeat("row\n", 300), "Diff preview truncated"},
		{"byte limit", strings.Repeat("x", 20000), "Diff preview truncated"},
		{"comparison bytes", strings.Repeat("x", 300000), "Diff preview omitted"},
		{"comparison lines", strings.Repeat("x\n", 5000), "Diff preview omitted"},
		{"non-text", "text\x00data", "non-text data"},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			workspace := test.TempDir()
			result, operationError := (Write{}).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": "file.txt", "content": scenario.content}))
			testutil.RequireNoError(test, operationError)
			if !strings.Contains(result.Text(), scenario.notice) || !strings.Contains(result.Text(), "full edit succeeded") || len(result.Text()) > fileDiffPreviewBytes+1024 {
				test.Fatalf("incorrect preview limit feedback (%d bytes): %.300s", len(result.Text()), result.Text())
			}
			actual, operationError := os.ReadFile(filepath.Join(workspace, "file.txt"))
			testutil.RequireNoError(test, operationError)
			if string(actual) != scenario.content {
				test.Fatal("preview limit truncated committed file content")
			}
		})
	}
}

func TestSmallEditInLargeFileKeepsAbsoluteHunkCoordinates(test *testing.T) {
	original := strings.Repeat("before\n", 100000) + "old\n" + strings.Repeat("after\n", 100000)
	updated := strings.Replace(original, "old\n", "new\n", 1)
	preview := renderFileEditDiff("large.txt", []byte(original), []byte(updated), true, fileDiffContextLines)
	if !strings.Contains(preview, "@@ -99998,7 +99998,7 @@") || !strings.Contains(preview, "-old\n+new\n") || strings.Contains(preview, "omitted") {
		test.Fatalf("large-file context or absolute line numbers are incorrect: %s", preview)
	}
}
