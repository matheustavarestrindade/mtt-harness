package tools

import (
	"context"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/taskstate"
)

type TaskState struct {
	Update func(context.Context, atom.Session, atom.TaskStateUpdate) (atom.TaskState, error)
}

func (TaskState) Name() string         { return "task_state" }
func (TaskState) Categories() []string { return []string{"task", "progress", "planning"} }
func (TaskState) Description() string {
	return "Persist the current session's TODO items and DOING activity in one atomic update. Use tracking only when multi-step work or progress feedback benefits the task; skip it for simple replies. Discover this definition before first use. TODO items have stable caller-chosen IDs, titles and pending/in_progress/done/cancelled status. Update only changed items by ID; existing order is kept and new IDs append. Completed items stay visible while unfinished work remains. When every stored item is done/cancelled, TODO and DOING clear together automatically. An empty todo array clears the list and its current activity; an explicitly supplied doing object can start activity without a list. doing:null clears activity. Omitted fields remain unchanged. DOING has a short title and a factual description of the current work, not internal deliberation. Every successful update resets the three-response refresh counter, including an unchanged reaffirmation. An overdue request asks for this update first; no need to resend unchanged TODO items. State survives restarts and is isolated from parent/child sessions. Cancellation/failure clears DOING but retains TODO; revert clears state and session deletion removes it. At most 32 stored items; IDs 1-64 ASCII letters/digits/-/_, item titles 1-160 characters, DOING title 1-120, description 1-1200. Invalid updates change nothing. Results are compact Updated/Cleared facts; the latest state is supplied separately in model context and UI events."
}
func (TaskState) InputSchema() atom.Schema {
	return schema(`{
		"type":"object","additionalProperties":false,"anyOf":[{"required":["todo"]},{"required":["doing"]}],
		"properties":{
			"todo":{"type":"array","maxItems":32,"description":"Optional partial item updates, merged by stable ID. Omitted preserves the list. Empty clears TODO and current DOING, unless this call supplies a new doing object. Existing items retain their order; new IDs append. Every item being done/cancelled automatically clears both fields. Maximum 32 stored items after merging.","items":{
				"type":"object","additionalProperties":false,"required":["id"],"properties":{
					"id":{"type":"string","minLength":1,"maxLength":64,"pattern":"^[A-Za-z0-9_-]+$","description":"Stable ID chosen by the model for this session, such as 1 or test-parser. Required on creation and updates; use the same ID to change an item. Duplicate IDs within one call are invalid."},
					"title":{"type":"string","minLength":1,"maxLength":160,"description":"Short non-blank task title, 1-160 characters. Required for a new ID; omitted on an existing ID preserves its title."},
					"status":{"type":"string","enum":["pending","in_progress","done","cancelled"],"description":"pending means not started, in_progress means underway, done means completed, cancelled means intentionally dropped. Omitted defaults to pending for new items and preserves an existing item's status. Mark only completed work done."}
				}
			}},
			"doing":{"type":["object","null"],"additionalProperties":false,"required":["title","description"],"description":"Optional replacement of the current activity. Omitted preserves it; null clears it. An object requires both fields. May be updated alone without resending TODO. Reaffirm unchanged activity when a refresh is due; do not invent progress.","properties":{
				"title":{"type":"string","minLength":1,"maxLength":120,"description":"Short non-blank overview of the current work, 1-120 characters."},
				"description":{"type":"string","minLength":1,"maxLength":1200,"description":"Factual explanation of the current step, goal, relevant constraints or blocker, 1-1200 characters. Keep it useful and concise; do not include private deliberation."}
			}}
		},
		"examples":[
			{"todo":[{"id":"1","title":"Inspect the parser","status":"in_progress"},{"id":"2","title":"Fix and verify parsing","status":"pending"}],"doing":{"title":"Inspecting the parser","description":"Reading the parsing path and its existing regression coverage."}},
			{"todo":[{"id":"1","status":"done"},{"id":"2","status":"in_progress"}],"doing":{"title":"Fixing parsing","description":"Applying the correction and running the relevant checks."}},
			{"todo":[],"doing":null}
		]
	}`)
}
func (TaskState) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}
func (tool TaskState) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	update, operationError := taskstate.DecodeUpdate(call.Input)
	if operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
	}
	session, found := harness.SessionFrom(operationContext)
	if !found || session.ID == "" {
		operationError = fmt.Errorf("task_state requires a session")
	}
	if operationError == nil && tool.Update == nil {
		operationError = fmt.Errorf("task state service is unavailable")
	}
	if operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
	}
	state, operationError := tool.Update(operationContext, session, update)
	if operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
	}
	result := "Updated"
	if !taskstate.Active(state) {
		result = "Cleared"
	}
	return atom.ToolResult{CallID: call.ID, Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: result}}}, nil
}
