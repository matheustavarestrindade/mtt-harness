package tools

import (
	"context"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// ReviewDecision is a registered, session-only lifecycle tool. The loop owns
// terminal decisions; the plugin applies a result after its source fences pass.
type ReviewDecision struct{ Intervene bool }

func (tool ReviewDecision) Name() string {
	if tool.Intervene {
		return "interfere"
	}
	return "continue"
}
func (ReviewDecision) Categories() []string { return []string{"review"} }
func (ReviewDecision) SessionOnly() bool    { return true }
func (ReviewDecision) Available(operationContext context.Context) (bool, error) {
	session, found := harness.SessionFrom(operationContext)
	return found && session.Execution != nil && session.Execution.DecisionRoot, nil
}
func (tool ReviewDecision) Description() string {
	if tool.Intervene {
		return "Finish this watcher review with a concrete correction for the main agent. message is required, 1-8192 characters. This records a decision; the harness applies it only while the reviewed task, session, configuration and permission request remain current. For a permission review it rejects that pending call. For context or completion review it submits identified runtime guidance; completion review can start a new main turn. This is terminal: do not call other tools after it."
	}
	return "Finish this watcher review and allow the normal flow to proceed. reason is an optional short explanation, at most 4096 characters; omitted/empty adds no explanation. For a permission review this approves the exact pending call once, subject to source revalidation. For context or completion review it records that no intervention is needed. This is terminal: do not call other tools after it."
}
func (tool ReviewDecision) InputSchema() atom.Schema {
	if tool.Intervene {
		return schema(`{"type":"object","additionalProperties":false,"properties":{"message":{"type":"string","minLength":1,"maxLength":8192,"description":"Concrete correction for the main agent; 1-8192 characters. Cite relevant supplied facts, without private reasoning."}},"required":["message"]}`)
	}
	return schema(`{"type":"object","additionalProperties":false,"properties":{"reason":{"type":"string","maxLength":4096,"default":"","description":"Optional brief explanation of the continue decision, at most 4096 characters. Empty means no explanation."}}}`)
}
func (ReviewDecision) Check(context.Context, atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}
func (ReviewDecision) Run(context.Context, atom.ToolCall) (atom.ToolResult, error) {
	return atom.ToolResult{}, fmt.Errorf("review decision requires a restricted session lifecycle")
}
