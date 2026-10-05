package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// FileActions is the model-facing file interface. One path gate owns a complete
// chain and its diagnostic snapshot; separate paths can run concurrently.
type FileActions struct{ lineNumbers bool }

func NewFileActions(lineNumbers bool) *FileActions { return &FileActions{lineNumbers: lineNumbers} }
func (FileActions) Name() string                   { return "file_actions" }
func (FileActions) Categories() []string           { return []string{"file"} }
func (FileActions) Check(context.Context, atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (fileTool FileActions) Description() string {
	description := "Read files, list directories, write, replace exact text, append or prepend using one ordered action chain on one workspace-resolved path. Edits require an explicit return choice: summary, diff, or read. Never returns a diff by default. Each action sees prior staged changes. A preliminary read call is not required when the relevant content is already known or the literal edit does not depend on existing content. Append/prepend supplied text or try a known exact replacement directly; select return read/diff for inspection and on_error read for missing context if it fails. Read first only when unknown structure, formatting, targets, or line positions affect the edit. All edits commit once by atomic rename, or none commit on failure; permission bits are preserved and other hard links keep old contents. Read/list actions and selected return output share a 200-line/16-KiB preview budget plus compact status/continuation notices. on_error may read or list the same path for diagnosis, but cannot mutate, retry, or hide failure. Failed chains discard intermediate previews and diagnose the original file snapshot. Choose return read when you want to inspect the finished text instead of calling another tool. A directory list gives direct children sorted by name, entry types and byte sizes, with an opaque continuation cursor. Full edit content is never truncated."
	if fileTool.lineNumbers {
		return description + " Read previews have absolute line-number display labels; labels and notices are not file content."
	}
	return description + " Read previews use plain text without line-number display labels; notices are not file content."
}

type fileActionSnapshot struct {
	content []byte
	exists  bool
	loaded  bool
}

func (fileTool FileActions) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	input, operationError := decodeFileActionsInput(call.Input)
	if operationError != nil {
		return fileActionsFailure(call, input.Path, operationError, "")
	}
	if operationError := operationContext.Err(); operationError != nil {
		return fileActionsFailure(call, input.Path, operationError, "")
	}
	path, operationError := harness.WorkspacePath(operationContext, input.Path)
	if operationError != nil {
		return fileActionsFailure(call, input.Path, operationError, "")
	}
	release, operationError := acquireFileEdit(operationContext, path)
	if operationError != nil {
		return fileActionsFailure(call, path, operationError, "")
	}
	defer release()
	var snapshot fileActionSnapshot
	var output string
	if input.mutates {
		output, operationError = fileTool.runFileActionTransaction(operationContext, path, input, &snapshot)
	} else {
		output, operationError = fileTool.runFileInspections(operationContext, path, input)
	}
	if operationError == nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: output}}}, nil
	}
	diagnostic := ""
	if input.OnError.Type != "" && operationContext.Err() == nil && !errors.Is(operationError, context.Canceled) && !errors.Is(operationError, context.DeadlineExceeded) {
		diagnostic = fileTool.renderFileActionRecovery(operationContext, path, input.OnError, snapshot)
	}
	return fileActionsFailure(call, path, operationError, diagnostic)
}

func fileActionsFailure(call atom.ToolCall, path string, cause error, diagnostic string) (atom.ToolResult, error) {
	operationError := fmt.Errorf("file_actions %q failed: %w\nThis call committed no file changes. Correct the failed action using available context or the diagnostic. Read/list only if information needed for the correction is still missing.", path, cause)
	result := atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}
	if diagnostic != "" {
		result.Content = []atom.Content{{Type: atom.Text, Text: diagnostic}}
	}
	return result, operationError
}

func (fileTool FileActions) runFileActionTransaction(operationContext context.Context, path string, input fileActionsInput, snapshot *fileActionSnapshot) (string, error) {
	information, operationError := os.Stat(path)
	if operationError != nil && !os.IsNotExist(operationError) {
		return "", operationError
	}
	if information != nil {
		if !information.Mode().IsRegular() {
			return "", fmt.Errorf("file actions require a regular file; use list to inspect a directory")
		}
		snapshot.content, operationError = os.ReadFile(path)
		if operationError != nil {
			return "", operationError
		}
		snapshot.exists = true
	}
	snapshot.loaded = true
	current, exists := snapshot.content, snapshot.exists
	results := newFileActionResults()
	var summaries []string
	for index, action := range input.Actions {
		if operationError := operationContext.Err(); operationError != nil {
			return "", operationError
		}
		if !exists && !(action.Operation == "write" && !action.hasBounds()) {
			return "", &fileActionFailure{index: index + 1, operation: action.Operation, cause: fmt.Errorf("target file does not exist; create it with a whole-file write first: %w", os.ErrNotExist)}
		}
		var summary string
		switch action.Operation {
		case "read":
			var text string
			text, operationError = results.read(operationContext, bytes.NewReader(current), action.fileTextSelection, fileTool.lineNumbers)
			if operationError == nil {
				results.add(fmt.Sprintf("Action %d read (file state at this step):", index+1), text)
			}
			summary = "read"
		case "write":
			current, summary, operationError = writeFileContent(current, *action.Content, action.lineRange)
			exists = true
		case "replace":
			mode := "first"
			if action.Mode != nil {
				mode = *action.Mode
			}
			var count int
			current, count, operationError = replaceFileText(current, *action.OldText, *action.NewText, mode, action.lineRange)
			summary = fmt.Sprintf("replace: %d match(es), mode %s", count, mode)
		case "append", "prepend":
			current = concatenateFileContent(current, []byte(*action.Content), action.Operation == "prepend")
			summary = fmt.Sprintf("%s: %d bytes", action.Operation, len(*action.Content))
		}
		if operationError != nil {
			return "", &fileActionFailure{index: index + 1, operation: action.Operation, cause: operationError}
		}
		summaries = append(summaries, fmt.Sprintf("%d. %s", index+1, summary))
	}
	changed := !snapshot.exists || !bytes.Equal(snapshot.content, current)
	switch input.Return.Type {
	case "read":
		text, operationError := results.read(operationContext, bytes.NewReader(current), input.Return.fileTextSelection, fileTool.lineNumbers)
		if operationError != nil {
			return "", fmt.Errorf("return read: %w", operationError)
		}
		results.add("Return read (final file):", text)
	case "diff":
		text := "No content changes."
		if changed {
			contextLines := fileDiffContextLines
			if input.Return.ContextLines != nil {
				contextLines = *input.Return.ContextLines
			}
			text = renderFileEditDiffWithin(path, snapshot.content, current, snapshot.exists, contextLines, results.budget)
		}
		results.consume(text)
		results.add("Return diff (net change):", text)
	}
	if operationError := operationContext.Err(); operationError != nil {
		return "", operationError
	}
	if changed {
		if operationError := replaceFileContentsAtomically(operationContext, path, information, current); operationError != nil {
			return "", operationError
		}
	}
	status := "Updated"
	if !snapshot.exists {
		status = "Created"
	} else if !changed {
		status = "Unchanged"
	}
	header := fmt.Sprintf("%s %q\nActions completed: %d. Lines: %d -> %d. Bytes: %d -> %d.\n%s", status, path, len(input.Actions), countFileLines(snapshot.content), countFileLines(current), len(snapshot.content), len(current), strings.Join(summaries, "\n"))
	if len(results.parts) > 0 {
		return header + "\n\n" + results.text(), nil
	}
	return header, nil
}

func concatenateFileContent(original, content []byte, prepend bool) []byte {
	updated := make([]byte, 0, len(original)+len(content))
	if prepend {
		updated = append(updated, content...)
		return append(updated, original...)
	}
	updated = append(updated, original...)
	return append(updated, content...)
}
