package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// FileActions stages an action chain for one or several targets under path gates.
// Targets prepare together; filesystem commits remain atomic per target.
type FileActions struct{ lineNumbers bool }

func NewFileActions(lineNumbers bool) *FileActions { return &FileActions{lineNumbers: lineNumbers} }
func (FileActions) Name() string                   { return "file_actions" }
func (FileActions) Categories() []string           { return []string{"file"} }
func (FileActions) Check(context.Context, atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (fileTool FileActions) Description() string {
	description := "Read, list, glob, write, replace, append, prepend or delete with one shared action chain on path or paths. Use paths to apply identical operations/text to up to 32 targets without repeating tool calls or content. Supply exactly one of path or paths. Edits require explicit return: summary, diff, or read. Never returns a diff by default. " +
		"Return only the selected output. With return, read gives final text, diff the net changes, list directory entries, summary a compact status. Single-target calls have no path banner, action log, line/byte totals or Return label. Multiple targets have the compact labels/counts described below. Intermediate read/list output is suppressed when return is present. Without return, inspections emit only requested data. For edit-and-inspect, use mutation actions and return.type=read; no preparatory or trailing read is needed for already known content. " +
		"Each target sees the ordered chain independently. All targets and selected output are prepared before any commit, under canonically ordered path gates. Preparation failures change nothing. Commits run in target order and are atomic per file, not across the batch: a commit failure stops later targets and reports the failed target and already committed target indexes. No automatic diagnostic reads, retries or rollback of committed targets run. Multiple-target read/list output uses short path labels; summary combines status counts. Do not repeat a whole batch blindly after a partial commit. Read first only when missing facts affect an edit. Replacements preserve permissions; deletion leaves other hard links intact. delete removes files, links themselves, or empty directories. Directory/link deletion requires a standalone delete with return summary. Never recursive, never follows the final link, never removes the workspace root. return read requires a final file. " +
		"Directory discovery defaults to compact rows: F regular file, D directory, L symlink, S other entry, followed by the quoted relative path. Metadata is opt-in through fields. list returns direct children. glob requires a relative Doublestar pattern and defaults to regular files: ** recurses, * and ? stay within a path component, character classes and {alternatives} are supported. Glob is case-sensitive, includes hidden entries, ignores no files implicitly, never traverses symlinks, and supports explicit exclude patterns and kind filtering. It returns matching paths, not file-content search or automatic edits. Directory discovery cannot mix with mutation/file-content actions. Results are sorted by case-sensitive relative path. limit defaults to 100, maximum 200; cursors continue the same root and query. All returned previews share 200 lines/16 KiB plus bounded factual continuation metadata. Full edits are never truncated."
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
	return fileTool.runFileActionTargets(operationContext, call, input)
}

func (fileTool FileActions) prepareFileActionTransaction(operationContext context.Context, path string, input fileActionsInput, snapshot *fileActionSnapshot, results *fileActionResults, plan *fileActionPlan) (string, error) {
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
		results.consume(output)
	}
	if operationError := operationContext.Err(); operationError != nil {
		return "", operationError
	}
	if changed && exists {
		replacement, operationError := prepareFileReplacement(operationContext, path, information, current)
		if operationError != nil {
			return "", &fileActionFailure{operation: "prepare commit", cause: operationError}
		}
		plan.commit = replacement.commit
		plan.discard = replacement.discard
	} else if changed {
		plan.commit = func(operationContext context.Context) error {
			if operationError := operationContext.Err(); operationError != nil {
				return operationError
			}
			return os.Remove(path)
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
