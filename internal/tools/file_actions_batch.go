package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type fileActionTarget struct{ requested, resolved string }

// Plans own staged bytes and temporary replacements until commit or disposal.
// Preparation cannot change a destination. Filesystem commits are atomic per
// entry, not across entries; a commit error reports prior committed indexes.
type fileActionPlan struct {
	path        string
	information os.FileInfo
	output      string
	commit      func(context.Context) error
	discard     func()
}

type fileBatchFailure struct {
	cause     error
	target    int
	committed []int
}

func (failure *fileBatchFailure) Unwrap() error { return failure.cause }
func (failure *fileBatchFailure) Error() string {
	text := fmt.Sprintf("%s\nTarget index: %d", failure.cause, failure.target)
	if len(failure.committed) > 0 {
		text += fmt.Sprintf("\nCommitted target indexes: %v", failure.committed)
	}
	return text
}

func failFileActionTarget(call atom.ToolCall, path string, cause error, information os.FileInfo, index int, multiple bool, committed []int) (atom.ToolResult, error) {
	result, operationError := failedFileOperation(call, path, "", cause, information)
	if !multiple {
		return result, operationError
	}
	failure := &fileBatchFailure{cause: operationError, target: index + 1, committed: append([]int(nil), committed...)}
	result.Error = failure.Error()
	return result, failure
}

func resolveFileActionTargets(operationContext context.Context, input fileActionsInput) ([]fileActionTarget, int, error) {
	targets := make([]fileActionTarget, 0, len(input.Paths))
	seen := make(map[string]bool, len(input.Paths))
	for index, path := range input.Paths {
		input.Path = path
		resolved, operationError := resolveFileActionPath(operationContext, input)
		if operationError != nil {
			return nil, index, operationError
		}
		if seen[resolved] {
			return nil, index, fmt.Errorf("duplicate resolved path %q", resolved)
		}
		for _, previous := range targets {
			if input.mutates && (filePathContains(previous.resolved, resolved) || filePathContains(resolved, previous.resolved)) {
				return nil, index, fmt.Errorf("overlapping mutation paths %q and %q", previous.requested, path)
			}
		}
		seen[resolved] = true
		targets = append(targets, fileActionTarget{requested: path, resolved: resolved})
	}
	return targets, 0, nil
}

func filePathContains(parent, child string) bool {
	relative, operationError := filepath.Rel(parent, child)
	return operationError == nil && relative != "." && relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// A canonical order prevents batches with opposite input orders deadlocking.
func acquireFileActionTargets(operationContext context.Context, targets []fileActionTarget) (func(), error) {
	paths := make([]string, len(targets))
	for index, target := range targets {
		paths[index] = target.resolved
	}
	slices.Sort(paths)
	var releases []func()
	releaseAll := func() {
		for index := len(releases) - 1; index >= 0; index-- {
			releases[index]()
		}
	}
	for _, path := range paths {
		release, operationError := acquireFileEdit(operationContext, path)
		if operationError != nil {
			releaseAll()
			return nil, operationError
		}
		releases = append(releases, release)
	}
	return releaseAll, nil
}

func (fileTool FileActions) runFileActionTargets(operationContext context.Context, call atom.ToolCall, input fileActionsInput) (atom.ToolResult, error) {
	multiple := len(input.Paths) > 1
	targets, failedIndex, operationError := resolveFileActionTargets(operationContext, input)
	if operationError != nil {
		return failFileActionTarget(call, input.Paths[failedIndex], operationError, nil, failedIndex, multiple, nil)
	}
	release, operationError := acquireFileActionTargets(operationContext, targets)
	if operationError != nil {
		return failedFileOperation(call, "", "", operationError, nil)
	}
	defer release()
	plans := make([]fileActionPlan, len(targets))
	defer func() {
		for _, plan := range plans {
			if plan.discard != nil {
				plan.discard()
			}
		}
	}()
	results := newFileActionResults()
	var outputs []string
	firstOmitted := 0
	for index, target := range targets {
		input.Path = target.requested
		// Only the budget crosses targets; inspection text belongs to one target.
		results.parts = nil
		plan := &plans[index]
		plan.path = target.resolved
		var snapshot fileActionSnapshot
		emit := !multiple || input.Return.Type == "summary" || results.budget.lines > 0 && results.budget.bytes > 0
		prefix := ""
		if multiple && emit && input.Return.Type != "summary" {
			if input.Return.Type != "diff" {
				prefix = fmt.Sprintf("%q\n", target.requested)
			}
			if len(outputs) > 0 {
				prefix = "\n" + prefix
			}
			if len(prefix) > results.budget.bytes || countFileLines([]byte(prefix)) > results.budget.lines {
				emit = false
				results.budget = filePreviewLimits{}
			} else {
				results.consume(prefix)
			}
		}
		if !emit && firstOmitted == 0 {
			firstOmitted = index + 1
		}
		switch {
		case len(input.Actions) == 1 && input.Actions[0].Operation == "delete":
			plan.output, operationError = fileTool.prepareFileDeletion(operationContext, target.resolved, input, &snapshot, results, plan)
		case input.mutates:
			plan.output, operationError = fileTool.prepareFileActionTransaction(operationContext, target.resolved, input, &snapshot, results, plan)
		default:
			plan.output, operationError = fileTool.runFileInspections(operationContext, target.resolved, input, &snapshot, results)
		}
		plan.information = snapshot.information
		if operationError != nil {
			return failFileActionTarget(call, target.resolved, operationError, snapshot.information, index, multiple, nil)
		}
		if emit {
			outputs = append(outputs, prefix+plan.output)
		}
	}
	failedIndex, committed, operationError := commitFileActionPlans(operationContext, plans)
	if operationError != nil {
		plan := plans[failedIndex]
		var actionFailure *fileActionFailure
		if !errors.As(operationError, &actionFailure) {
			operationError = &fileActionFailure{operation: "commit", cause: operationError}
		}
		return failFileActionTarget(call, plan.path, operationError, plan.information, failedIndex, multiple, committed)
	}
	output := strings.Join(outputs, "")
	if multiple && input.Return.Type == "summary" {
		output = summarizeFileActionPlans(plans)
	}
	if firstOmitted > 0 {
		output += fmt.Sprintf("\n[Output limit reached. Next target index: %d.]", firstOmitted)
	}
	return atom.ToolResult{CallID: call.ID, Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: output}}}, nil
}

func commitFileActionPlans(operationContext context.Context, plans []fileActionPlan) (int, []int, error) {
	var committed []int
	for index, plan := range plans {
		if operationError := operationContext.Err(); operationError != nil {
			return index, committed, operationError
		}
		if plan.commit == nil {
			continue
		}
		if operationError := plan.commit(operationContext); operationError != nil {
			return index, committed, operationError
		}
		committed = append(committed, index+1)
	}
	return 0, committed, nil
}

func summarizeFileActionPlans(plans []fileActionPlan) string {
	counts := map[string]int{}
	for _, plan := range plans {
		counts[plan.output]++
	}
	var summaries []string
	for _, status := range []string{"Created", "Updated", "Deleted", "Unchanged", "OK"} {
		if count := counts[status]; count > 0 {
			summaries = append(summaries, fmt.Sprintf("%s %d", status, count))
		}
	}
	return strings.Join(summaries, "; ")
}
