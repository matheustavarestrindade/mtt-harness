package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestStagedFormattingPreservesUnstagedFiles(test *testing.T) {
	repositoryPath := newTestRepository(test)
	stagedPath := "source with spaces.go"
	unformattedSource := []byte("package example\nfunc Answer( )int{return 42}\n")
	writeTestFile(test, repositoryPath, stagedPath, unformattedSource)
	runTestGit(test, repositoryPath, "add", "--", stagedPath)
	unstagedContent := []byte("unrelated work\n")
	writeTestFile(test, repositoryPath, "notes.txt", unstagedContent)
	if operationError := formatRepository(repositoryPath, false); operationError != nil {
		test.Fatal(operationError)
	}
	expectedSource := []byte("package example\n\nfunc Answer() int { return 42 }\n")
	stagedContent := runTestGit(test, repositoryPath, "show", ":"+stagedPath)
	if !bytes.Equal(stagedContent, expectedSource) {
		test.Fatalf("index was not formatted: %s", stagedContent)
	}
	workingContent, operationError := os.ReadFile(filepath.Join(repositoryPath, stagedPath))
	if operationError != nil || !bytes.Equal(workingContent, stagedContent) {
		test.Fatal("formatted index differs from working file", operationError)
	}
	if _, operationError := runGit(repositoryPath, nil, "show", ":notes.txt"); operationError == nil {
		test.Fatal("formatter staged unrelated work")
	}
}

func TestPartiallyStagedFormattingLeavesIndexAndWorktreeIntact(test *testing.T) {
	repositoryPath := newTestRepository(test)
	unformattedSource := []byte("package example\nfunc Answer( )int{return 42}\n")
	writeTestFile(test, repositoryPath, "example.go", unformattedSource)
	runTestGit(test, repositoryPath, "add", "example.go")
	workingSource := append(append([]byte(nil), unformattedSource...), []byte("// unstaged work\n")...)
	writeTestFile(test, repositoryPath, "example.go", workingSource)
	if operationError := formatRepository(repositoryPath, false); operationError == nil {
		test.Fatal("partially staged formatting silently succeeded")
	}
	if !bytes.Equal(runTestGit(test, repositoryPath, "show", ":example.go"), unformattedSource) {
		test.Fatal("failed hook changed the index")
	}
	workingContent, operationError := os.ReadFile(filepath.Join(repositoryPath, "example.go"))
	if operationError != nil || !bytes.Equal(workingContent, workingSource) {
		test.Fatal("failed hook changed unstaged work", operationError)
	}
}

func TestAllFormattingDoesNotStageChanges(test *testing.T) {
	repositoryPath := newTestRepository(test)
	originalJSON := []byte(`{"providers":[]}`)
	writeTestFile(test, repositoryPath, "providers.json", originalJSON)
	runTestGit(test, repositoryPath, "add", "providers.json")
	if operationError := formatRepository(repositoryPath, true); operationError != nil {
		test.Fatal(operationError)
	}
	if !bytes.Equal(runTestGit(test, repositoryPath, "show", ":providers.json"), originalJSON) {
		test.Fatal("--all changed the index")
	}
}

func TestStagedFormattingDoesNotStagePermissionChanges(test *testing.T) {
	repositoryPath := newTestRepository(test)
	writeTestFile(test, repositoryPath, "example.go", []byte("package example\nfunc Answer( )int{return 42}\n"))
	runTestGit(test, repositoryPath, "add", "example.go")
	if operationError := os.Chmod(filepath.Join(repositoryPath, "example.go"), 0755); operationError != nil {
		test.Fatal(operationError)
	}
	if operationError := formatRepository(repositoryPath, false); operationError != nil {
		test.Fatal(operationError)
	}
	indexEntry := runTestGit(test, repositoryPath, "ls-files", "--stage", "example.go")
	if !bytes.HasPrefix(indexEntry, []byte("100644 ")) {
		test.Fatalf("formatter staged an unstaged permission change: %s", indexEntry)
	}
}

func newTestRepository(test *testing.T) string {
	test.Helper()
	repositoryPath := test.TempDir()
	runTestGit(test, repositoryPath, "init", "--quiet")
	return repositoryPath
}

func writeTestFile(test *testing.T, repositoryPath, relativePath string, content []byte) {
	test.Helper()
	if operationError := os.WriteFile(filepath.Join(repositoryPath, relativePath), content, 0644); operationError != nil {
		test.Fatal(operationError)
	}
}

func runTestGit(test *testing.T, repositoryPath string, arguments ...string) []byte {
	test.Helper()
	output, operationError := runGit(repositoryPath, nil, arguments...)
	if operationError != nil {
		test.Fatal(operationError)
	}
	return output
}
