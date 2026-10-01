// Command format formats staged code before commits, or tracked working files
// with --all. Staged mode never includes unstaged edits in a commit.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type formattingChange struct {
	path      string
	original  []byte
	formatted []byte
	mode      os.FileMode
	indexMode string
}

func main() {
	allFiles := flag.Bool("all", false, "format tracked working files without staging them")
	flag.Parse()
	repositoryPath, operationError := runGit("", nil, "rev-parse", "--show-toplevel")
	if operationError == nil {
		operationError = formatRepository(strings.TrimSpace(string(repositoryPath)), *allFiles)
	}
	if operationError != nil {
		fmt.Fprintln(os.Stderr, "format:", operationError)
		os.Exit(1)
	}
}

func formatRepository(repositoryPath string, allFiles bool) error {
	arguments := []string{"diff", "--cached", "--name-only", "--diff-filter=ACMR", "-z"}
	if allFiles {
		arguments = []string{"ls-files", "-z"}
	}
	fileList, operationError := runGit(repositoryPath, nil, arguments...)
	if operationError != nil {
		return operationError
	}
	var changes []formattingChange
	for _, relativePath := range strings.Split(string(fileList), "\x00") {
		if !supportedFormat(relativePath) {
			continue
		}
		absolutePath := filepath.Join(repositoryPath, relativePath)
		fileInfo, operationError := os.Lstat(absolutePath)
		if os.IsNotExist(operationError) && allFiles {
			continue
		}
		if operationError != nil {
			return operationError
		}
		if !fileInfo.Mode().IsRegular() {
			continue
		}
		workingContent, operationError := os.ReadFile(absolutePath)
		if operationError != nil {
			return operationError
		}
		selectedContent := workingContent
		indexMode := ""
		if !allFiles {
			indexEntry, operationError := runGit(repositoryPath, nil, "ls-files", "--stage", "-z", "--", relativePath)
			if operationError != nil {
				return operationError
			}
			indexMode, _, _ = strings.Cut(string(indexEntry), " ")
			if indexMode != "100644" && indexMode != "100755" {
				continue
			}
			selectedContent, operationError = runGit(repositoryPath, nil, "show", ":"+relativePath)
			if operationError != nil {
				return operationError
			}
		}
		formattedContent, operationError := formatContent(repositoryPath, relativePath, selectedContent)
		if operationError != nil {
			return fmt.Errorf("%s: %w", relativePath, operationError)
		}
		if bytes.Equal(selectedContent, formattedContent) {
			continue
		}
		// Validate the whole batch before writing anything. Formatting must never
		// silently stage or overwrite the unstaged part of a partially staged file.
		if !allFiles && !bytes.Equal(workingContent, selectedContent) {
			return fmt.Errorf("%s needs formatting and has unstaged changes; run go run ./scripts/format --all and stage the intended hunks", relativePath)
		}
		changes = append(changes, formattingChange{path: relativePath, original: workingContent, formatted: formattedContent, mode: fileInfo.Mode(), indexMode: indexMode})
	}
	for _, change := range changes {
		absolutePath := filepath.Join(repositoryPath, change.path)
		currentContent, operationError := os.ReadFile(absolutePath)
		if operationError != nil {
			return operationError
		}
		if !bytes.Equal(currentContent, change.original) {
			return fmt.Errorf("%s changed during formatting; retry after the edit completes", change.path)
		}
		if operationError := os.WriteFile(absolutePath, change.formatted, change.mode.Perm()); operationError != nil {
			return operationError
		}
		if !allFiles {
			blobID, operationError := runGit(repositoryPath, change.formatted, "hash-object", "-w", "--stdin")
			if operationError != nil {
				return operationError
			}
			_, operationError = runGit(repositoryPath, nil, "update-index", "--cacheinfo", change.indexMode, strings.TrimSpace(string(blobID)), change.path)
			if operationError != nil {
				return operationError
			}
		}
		fmt.Println("formatted", change.path)
	}
	return nil
}

func runGit(repositoryPath string, input []byte, arguments ...string) ([]byte, error) {
	command := exec.Command("git", arguments...)
	command.Dir = repositoryPath
	command.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, operationError := command.Output()
	if operationError != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(arguments, " "), operationError, strings.TrimSpace(stderr.String()))
	}
	return output, nil
}
