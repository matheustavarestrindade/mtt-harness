package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	inputSchemaValidation "github.com/matheustavarestrindade/mtt-harness/internal/molecule/schema"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func createGlobFixture(test *testing.T) string {
	test.Helper()
	workspace := test.TempDir()
	for _, path := range []string{"main.go", "src/main.go", "src/main_test.go", "src/web/App.svelte", "src/web/app.ts", ".hidden/private.go", "vendor/skip.go", "src/node_modules/skip.go", "docs/readme.md", "docs/sample.txt", "weird/[name].go"} {
		fullPath := filepath.Join(workspace, filepath.FromSlash(path))
		testutil.RequireNoError(test, os.MkdirAll(filepath.Dir(fullPath), 0o700))
		testutil.RequireNoError(test, os.WriteFile(fullPath, []byte(path), 0o600))
	}
	return workspace
}

func globOutputPaths(test *testing.T, text string) []string {
	test.Helper()
	var paths []string
	for _, row := range strings.Split(text, "\n") {
		if row == "" || strings.HasPrefix(row, "[") || strings.HasPrefix(row, "(") {
			continue
		}
		_, quoted, found := strings.Cut(row, " ")
		if !found {
			test.Fatalf("invalid glob row: %q", row)
		}
		quoted, _, _ = strings.Cut(quoted, "\t")
		path, operationError := strconv.Unquote(quoted)
		testutil.RequireNoError(test, operationError)
		paths = append(paths, path)
	}
	return paths
}

func TestGlobPatternsAgreeWithPinnedLibraryTraversal(test *testing.T) {
	workspace := createGlobFixture(test)
	for _, pattern := range []string{"**/*.go", "{src,docs}/**/*.{go,md}", "src/*_test.go", "src/**", "src/**/", `weird/\[name\].go`, "src/main.go", "no-such-directory/*.go", "**/[am]*.{go,ts}"} {
		test.Run(pattern, func(test *testing.T) {
			for _, kind := range []string{"file", "directory", "all"} {
				call := fileCall(test, map[string]any{"path": ".", "actions": []any{map[string]any{"op": "glob", "pattern": pattern, "kind": kind}}})
				fileTool := NewFileActions(false)
				testutil.RequireNoError(test, inputSchemaValidation.Validate(fileTool.InputSchema().JSON, call.Input))
				result, operationError := fileTool.Run(harness.WithWorkspace(context.Background(), workspace), call)
				testutil.RequireNoError(test, operationError)
				matches, operationError := doublestar.Glob(os.DirFS(workspace), pattern, doublestar.WithNoFollow(), doublestar.WithFailOnIOErrors())
				testutil.RequireNoError(test, operationError)
				var expected []string
				for _, match := range matches {
					if match == "." {
						continue
					}
					information, operationError := os.Lstat(filepath.Join(workspace, match))
					testutil.RequireNoError(test, operationError)
					if globKindMatches(kind, information.Mode()) {
						expected = append(expected, match)
					}
				}
				slices.Sort(expected)
				actual := globOutputPaths(test, result.Text())
				if !slices.Equal(actual, expected) {
					test.Fatalf("%s %s: got %v, library gives %v", pattern, kind, actual, expected)
				}
			}
		})
	}
}

func TestGlobExclusionsAndMetadata(test *testing.T) {
	workspace := createGlobFixture(test)
	result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{
		"path": ".", "actions": []any{map[string]any{"op": "glob", "pattern": "**/*.go", "exclude": []string{"**/node_modules/**", "vendor/**", ".hidden/**"}, "fields": []string{"size"}}},
	}))
	testutil.RequireNoError(test, operationError)
	for _, path := range globOutputPaths(test, result.Text()) {
		if strings.Contains(path, "skip.go") || strings.Contains(path, "private.go") {
			test.Fatalf("excluded path returned: %s", path)
		}
		information, operationError := os.Stat(filepath.Join(workspace, path))
		testutil.RequireNoError(test, operationError)
		if !strings.Contains(result.Text(), fmt.Sprintf("F %q\tsize=%d\n", path, information.Size())) {
			test.Fatal("requested glob metadata differs from the file system")
		}
	}
	if len(globOutputPaths(test, result.Text())) != 4 {
		test.Fatalf("unexpected filtered glob: %s", result.Text())
	}
}

func TestGlobNeverTraversesLiteralOrWildcardSymlinks(test *testing.T) {
	workspace := test.TempDir()
	outside := test.TempDir()
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(outside, "secret.go"), []byte("not visible"), 0o600))
	testutil.RequireNoError(test, os.Symlink(outside, filepath.Join(workspace, "escape")))
	testutil.RequireNoError(test, os.Mkdir(filepath.Join(workspace, "inside"), 0o700))
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, "inside", "local.go"), nil, 0o600))
	testutil.RequireNoError(test, os.Symlink("inside", filepath.Join(workspace, "alias")))
	for _, pattern := range []string{"escape/*.go", "escape/**/*.go", "alias/*.go", "{escape,alias}/**/*.go", "**/*.go"} {
		result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": ".", "actions": []any{map[string]any{"op": "glob", "pattern": pattern}}}))
		testutil.RequireNoError(test, operationError)
		paths := globOutputPaths(test, result.Text())
		if pattern == "**/*.go" {
			if !slices.Equal(paths, []string{"inside/local.go"}) {
				test.Fatalf("recursive glob followed a link: %v", paths)
			}
		} else if len(paths) != 0 {
			test.Fatalf("literal prefix followed a link: %s: %v", pattern, paths)
		}
	}
	result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": ".", "actions": []any{map[string]any{"op": "glob", "pattern": "*", "kind": "link"}}}))
	testutil.RequireNoError(test, operationError)
	if result.Text() != "L \"alias\"\nL \"escape\"\n" {
		test.Fatalf("link entries were lost: %q", result.Text())
	}
}

func TestGlobPaginationIsSortedBoundedAndComplete(test *testing.T) {
	workspace := test.TempDir()
	for index := 257; index >= 0; index-- {
		directory := filepath.Join(workspace, fmt.Sprintf("folder-%d", index%4))
		testutil.RequireNoError(test, os.MkdirAll(directory, 0o700))
		testutil.RequireNoError(test, os.WriteFile(filepath.Join(directory, fmt.Sprintf("f-%03d.txt", index)), nil, 0o600))
	}
	options := fileGlobOptions{Pattern: "**/*.txt"}
	entries, operationError := collectGlobPage(context.Background(), workspace, options, "", 13, nil)
	testutil.RequireNoError(test, operationError)
	if len(entries) != 14 {
		test.Fatal("glob retained more than a bounded page")
	}
	var paths []string
	cursor := ""
	for {
		result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": ".", "actions": []any{map[string]any{"op": "glob", "pattern": options.Pattern, "limit": 13, "cursor": cursor}}}))
		testutil.RequireNoError(test, operationError)
		paths = append(paths, globOutputPaths(test, result.Text())...)
		start := strings.Index(result.Text(), `{"cursor":`)
		if start < 0 {
			break
		}
		end := strings.Index(result.Text()[start:], "}")
		var continuation struct {
			Cursor string `json:"cursor"`
		}
		testutil.RequireNoError(test, json.Unmarshal([]byte(result.Text()[start:start+end+1]), &continuation))
		if continuation.Cursor == cursor {
			test.Fatal("glob continuation made no progress")
		}
		cursor = continuation.Cursor
	}
	if len(paths) != 258 || !slices.IsSorted(paths) {
		test.Fatalf("glob pagination lost paths or ordering: %d", len(paths))
	}
	for index := 1; index < len(paths); index++ {
		if paths[index] == paths[index-1] {
			test.Fatal("glob repeated a path")
		}
	}
}

func TestGlobValidationAndCancellationAreFactualFailures(test *testing.T) {
	workspace := createGlobFixture(test)
	for _, input := range []string{
		`{"path":".","actions":[{"op":"glob"}]}`,
		`{"path":".","actions":[{"op":"glob","pattern":"["}]}`,
		`{"path":".","actions":[{"op":"glob","pattern":"../**"}]}`,
		`{"path":".","actions":[{"op":"glob","pattern":"/etc/*"}]}`,
		`{"path":".","actions":[{"op":"glob","pattern":"**","kind":"unknown"}]}`,
		`{"path":".","actions":[{"op":"glob","pattern":"**","exclude":["["]}]}`,
		`{"path":".","actions":[{"op":"glob","pattern":"**"},{"op":"write","content":"bad"}],"return":{"type":"summary"}}`,
	} {
		result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), atom.ToolCall{Input: []byte(input)})
		if operationError == nil || result.Status != atom.StatusError || len(result.Content) != 0 {
			test.Fatalf("invalid glob accepted: %s", input)
		}
	}
	operationContext, cancel := context.WithCancel(context.Background())
	root, operationError := os.OpenRoot(workspace)
	testutil.RequireNoError(test, operationError)
	defer root.Close()
	operationError = walkGlobDirectory(operationContext, root, "", -1, func(*os.Root, string, os.DirEntry) (bool, error) { cancel(); return true, nil })
	if !errors.Is(operationError, context.Canceled) {
		test.Fatalf("glob traversal ignored cancellation: %v", operationError)
	}
}

func TestGlobUsesSelectedRootsAndOneBatchOutputBudget(test *testing.T) {
	workspace := test.TempDir()
	for _, directory := range []string{"first", "second"} {
		testutil.RequireNoError(test, os.Mkdir(filepath.Join(workspace, directory), 0o700))
		for index := range 240 {
			testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, directory, fmt.Sprintf("f-%03d.go", index)), nil, 0o600))
		}
	}
	result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{
		"paths": []string{"first", "second"}, "actions": []any{map[string]any{"op": "glob", "pattern": "*.go", "limit": 200}},
	}))
	testutil.RequireNoError(test, operationError)
	if !strings.HasPrefix(result.Text(), "\"first\"\nF \"f-000.go\"\n") || strings.Count(result.Text(), "F ") != 199 || !strings.Contains(result.Text(), "Next target index: 2") {
		test.Fatalf("glob broke batch attribution or limits: %s", result.Text())
	}
}

func TestGlobIncludesHiddenAndGitignoredFilesUnlessExcluded(test *testing.T) {
	workspace := test.TempDir()
	testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, ".gitignore"), []byte("ignored.go\n"), 0o600))
	for _, name := range []string{"ignored.go", ".hidden.go"} {
		testutil.RequireNoError(test, os.WriteFile(filepath.Join(workspace, name), nil, 0o600))
	}
	result, operationError := NewFileActions(false).Run(harness.WithWorkspace(context.Background(), workspace), fileCall(test, map[string]any{"path": ".", "actions": []any{map[string]any{"op": "glob", "pattern": "*.go"}}}))
	testutil.RequireNoError(test, operationError)
	if result.Text() != "F \".hidden.go\"\nF \"ignored.go\"\n" {
		test.Fatalf("glob silently filtered entries: %q", result.Text())
	}
}

func TestGlobCursorIsBoundToRootAndQuery(test *testing.T) {
	root := test.TempDir()
	options := fileGlobOptions{Pattern: "**/*.go", Exclude: []string{"vendor/**"}}
	cursor := encodeGlobCursor(root, options, "src/file.go")
	after, operationError := globCursorPosition(root, options, cursor)
	testutil.RequireNoError(test, operationError)
	if after != "src/file.go" {
		test.Fatal("cursor lost its relative path")
	}
	for _, different := range []fileGlobOptions{{Pattern: "**/*.md"}, {Pattern: options.Pattern}, {Pattern: options.Pattern, Exclude: options.Exclude, Kind: "all"}} {
		if _, operationError := globCursorPosition(root, different, cursor); operationError == nil {
			test.Fatal("cursor accepted a different query")
		}
	}
	if _, operationError := globCursorPosition(test.TempDir(), options, cursor); operationError == nil {
		test.Fatal("cursor accepted a different root")
	}
	options.Kind = "file"
	_, operationError = globCursorPosition(root, options, cursor)
	testutil.RequireNoError(test, operationError)
}
