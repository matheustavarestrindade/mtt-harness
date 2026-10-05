package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/matheustavarestrindade/mtt-harness/atom"
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
	description := "Read, list, write, replace, append, prepend or delete using one ordered action chain on one workspace-resolved path. Edits require an explicit return choice: summary, diff, or read. Never returns a diff by default. " +
		"Return only the selected output. When return is present, it is the entire successful response: read gives final file text, diff gives the net diff, list gives directory entries, summary gives one compact status. No path banners, action logs, line/byte totals or Return labels accompany read/diff/list. Read/list actions still execute, but their intermediate output is suppressed when return is present. Without return, read/list actions emit only their requested data. For edit-and-inspect, use mutation actions and return.type=read; no preparatory or trailing read is needed for already known content. " +
		"Each action sees prior staged changes. Read first only when unknown structure, formatting, targets, or line positions affect the edit. Replacements commit by atomic rename; a final deletion uses unlink/rmdir after validation. Failed calls commit nothing, stop immediately and return only the failed operation, cause and observed modification time when available. No diagnostic reads or retries run. Permission bits are preserved for replacements. Deleting a file leaves other hard links intact. delete removes regular files, symbolic links themselves, or empty directories; directories and symbolic links require a standalone delete with return summary. Deletion never follows the final link, never removes a directory recursively, and cannot remove the workspace root. return read requires a final file to exist. " +
		"Listings default to compact rows: F means regular file, D directory, L symbolic link, S other entry, followed by the quoted name. Metadata is opt-in through fields: size (bytes), permissions (octal mode), owner/group (numeric IDs), modified (UTC time). No metadata is added by default. Direct children, including hidden entries, are sorted by case-sensitive name without following symlinks. All returned previews share 200 lines/16 KiB, plus bounded factual continuation metadata. Full edits are never truncated."
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
	path, operationError := resolveFileActionPath(operationContext, input)
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
	if len(input.Actions) == 1 && input.Actions[0].Operation == "delete" {
		output, operationError = fileTool.runFileDeletion(operationContext, path, input, &snapshot)
	} else if input.mutates {
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
	statPath := os.Stat
	if input.deletes {
		statPath = os.Lstat
	}
	information, operationError := statPath(path)
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
	for index, action := range input.Actions {
		if operationError := operationContext.Err(); operationError != nil {
			return "", &fileActionFailure{index: index + 1, operation: action.Operation, cause: operationError}
		}
		if !exists && !(action.Operation == "write" && !action.hasBounds()) {
			return "", &fileActionFailure{index: index + 1, operation: action.Operation, cause: os.ErrNotExist}
		}
		switch action.Operation {
		case "read":
			_, operationError = newFileActionResults().read(operationContext, bytes.NewReader(current), action.fileTextSelection, fileTool.lineNumbers)
		case "write":
			current, _, operationError = writeFileContent(current, *action.Content, action.lineRange)
			exists = true
		case "replace":
			mode := "first"
			if action.Mode != nil {
				mode = *action.Mode
			}
			current, _, operationError = replaceFileText(current, *action.OldText, *action.NewText, mode, action.lineRange)
		case "append", "prepend":
			current = concatenateFileContent(current, []byte(*action.Content), action.Operation == "prepend")
		case "delete":
			current, exists = nil, false
		}
		if operationError != nil {
			return "", &fileActionFailure{index: index + 1, operation: action.Operation, cause: operationError}
		}
	}
	changed := snapshot.exists != exists || exists && !bytes.Equal(snapshot.content, current)
	output := "Unchanged"
	if changed {
		switch {
		case !exists:
			output = "Deleted"
		case !snapshot.exists:
			output = "Created"
		default:
			output = "Updated"
		}
	}
	switch input.Return.Type {
	case "read":
		if !exists {
			return "", &fileActionFailure{operation: "return read", cause: os.ErrNotExist}
		}
		text, operationError := results.read(operationContext, bytes.NewReader(current), input.Return.fileTextSelection, fileTool.lineNumbers)
		if operationError != nil {
			return "", &fileActionFailure{operation: "return read", cause: operationError}
		}
		output = text
	case "diff":
		output = "No content changes."
		if changed {
			contextLines := fileDiffContextLines
			if input.Return.ContextLines != nil {
				contextLines = *input.Return.ContextLines
			}
			output = renderFileStateDiffWithin(path, snapshot.content, current, snapshot.exists, exists, contextLines, results.budget)
		}
	}
	if operationError := operationContext.Err(); operationError != nil {
		return "", operationError
	}
	if changed {
		if exists {
			operationError = replaceFileContentsAtomically(operationContext, path, information, current)
		} else {
			operationError = os.Remove(path)
		}
		if operationError != nil {
			return "", &fileActionFailure{operation: "commit", cause: operationError}
		}
	}
	return output, nil
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
