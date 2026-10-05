package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// FileActions is the model-facing file interface. One path gate owns a complete
// chain and its file snapshots; separate paths can run concurrently.
type FileActions struct{ lineNumbers bool }

func NewFileActions(lineNumbers bool) *FileActions { return &FileActions{lineNumbers: lineNumbers} }
func (FileActions) Name() string                   { return "file_actions" }
func (FileActions) Categories() []string           { return []string{"file"} }
func (FileActions) Check(context.Context, atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (fileTool FileActions) Description() string {
	description := "Read files, list directories, write, replace exact text, append or prepend using one ordered action chain on one workspace-resolved path. Edits require an explicit return choice: summary, diff, or read. Never returns a diff by default. " +
		"Request each file state/range only once. A read/list action already contributes its own output; return adds a separate final preview and does not replace or deduplicate action output. For an edit followed by final-text inspection, put only the edits in actions and choose return.type=read. Do not also append op=read for the same final content: that prints it twice and consumes the shared preview budget twice. For read/list-only calls, use the action and omit return. Explicit reads in an editing chain are for deliberately distinct intermediate states or ranges, not to enable return. " +
		"Each action sees prior staged changes. A preliminary read call is not required when the relevant content is already known or the literal edit does not depend on existing content. Append/prepend supplied text or try a known exact replacement directly; select return read/diff for inspection. Read first only when unknown structure, formatting, targets, or line positions affect the edit. All edits commit once by atomic rename, or none commit on failure; permission bits are preserved and other hard links keep old contents. The first failure stops the call immediately: no later actions, final preview, diagnostic reads/listing or retries run. Failed calls discard all intermediate output and report one factual error identifying the failed operation and cause, with observed last-modified time when available. on_error is not a supported input. Read/list actions and selected return output share a 200-line/16-KiB preview budget plus compact status/continuation metadata. A directory list gives direct children sorted by name, entry types and byte sizes, with an opaque continuation cursor. Full edit content is never truncated."
	if fileTool.lineNumbers {
		return description + " Read previews have absolute line-number display labels; labels and notices are not file content."
	}
	return description + " Read previews use plain text without line-number display labels; notices are not file content."
}

type fileActionSnapshot struct {
	content     []byte
	exists      bool
	information os.FileInfo
}

func (fileTool FileActions) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	input, operationError := decodeFileActionsInput(call.Input)
	if operationError != nil {
		return failedFileOperation(call, input.Path, "", operationError, nil)
	}
	if operationError := operationContext.Err(); operationError != nil {
		return failedFileOperation(call, input.Path, "", operationError, nil)
	}
	path, operationError := harness.WorkspacePath(operationContext, input.Path)
	if operationError != nil {
		return failedFileOperation(call, input.Path, "", operationError, nil)
	}
	release, operationError := acquireFileEdit(operationContext, path)
	if operationError != nil {
		return failedFileOperation(call, path, "", operationError, nil)
	}
	defer release()
	var snapshot fileActionSnapshot
	var output string
	if input.mutates {
		output, operationError = fileTool.runFileActionTransaction(operationContext, path, input, &snapshot)
	} else {
		output, operationError = fileTool.runFileInspections(operationContext, path, input, &snapshot)
	}
	if operationError == nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: output}}}, nil
	}
	return failedFileOperation(call, path, "", operationError, snapshot.information)
}

func (fileTool FileActions) runFileActionTransaction(operationContext context.Context, path string, input fileActionsInput, snapshot *fileActionSnapshot) (string, error) {
	information, operationError := os.Stat(path)
	snapshot.information = information
	if operationError != nil && !os.IsNotExist(operationError) {
		return "", &fileActionFailure{index: 1, operation: input.Actions[0].Operation, cause: operationError}
	}
	if information != nil {
		if !information.Mode().IsRegular() {
			return "", &fileActionFailure{index: 1, operation: input.Actions[0].Operation, cause: fmt.Errorf("target is not a regular file")}
		}
		snapshot.content, operationError = os.ReadFile(path)
		if operationError != nil {
			return "", &fileActionFailure{index: 1, operation: input.Actions[0].Operation, cause: operationError}
		}
		snapshot.exists = true
	}
	current, exists := snapshot.content, snapshot.exists
	results := newFileActionResults()
	var summaries []string
	for index, action := range input.Actions {
		if operationError := operationContext.Err(); operationError != nil {
			return "", &fileActionFailure{index: index + 1, operation: action.Operation, cause: operationError}
		}
		if !exists && !(action.Operation == "write" && !action.hasBounds()) {
			return "", &fileActionFailure{index: index + 1, operation: action.Operation, cause: os.ErrNotExist}
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
			return "", &fileActionFailure{operation: "return read", cause: operationError}
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
			return "", &fileActionFailure{operation: "commit", cause: operationError}
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
