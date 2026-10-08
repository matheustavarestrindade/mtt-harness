package spacedrepetition

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type instructionTool struct{ plugin *Plugin }

func (tool *instructionTool) Name() string { return "remember_instructions" }
func (tool *instructionTool) Categories() []string {
	return []string{"instructions", "context", "memory"}
}
func (tool *instructionTool) Check(context.Context, atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}
func (tool *instructionTool) Description() string {
	return "Recover applicable instructions when the user repeatedly reports broad instruction drift across task scope, stack, database choices, writing style, formatting, or confirmation behavior. Describe the recurring issues and use known conversation facts; handle a specific isolated correction directly. This runs a bounded workspace worker which reads the startup instructions, recent user messages, and relevant workspace memories at medium compression, then prepares focused high-level guidance. It waits for preparation and reports ready only after the report is saved. Guidance is injected after this complete tool group at a model-request boundary when it fits; ready does not claim the next request has already been sent. It does not change memory notes. If memory is unavailable, recovery uses current instructions and user evidence. The configured worker model consumes workspace-agent usage. Session cancellation, revert, deletion, or the configured timeout stops the worker and fences stale publication. Apply the guidance naturally; do not narrate the worker or quote internal reminder tags to the user. Replayed identical call IDs reuse the recorded outcome instead of repeating model calls."
}
func (tool *instructionTool) InputSchema() atom.Schema {
	return atom.Schema{JSON: json.RawMessage(`{"type":"object","additionalProperties":false,"required":["reason"],"properties":{"reason":{"type":"string","minLength":1,"maxLength":4096,"description":"Concrete description of repeated, broad instruction-following problems and their topics. Required; 1 to 4096 UTF-8 bytes after whitespace validation. Include observed corrections and missing behavior, not invented rules. An empty or whitespace-only value is invalid."}}}`)}
}
func (tool *instructionTool) SearchDocument() string {
	return "Recover instructions and relevant workspace preferences after repeated user complaints about task follow-through, stack choices, database conventions, communication, formatting, or permission/confirmation scope. Example: the user repeatedly says the agent ignores the prescribed stack and writing style. Search workspace memory at medium compression and prepare focused guidance for the next model request."
}

func (tool *instructionTool) Available(operationContext context.Context) (bool, error) {
	session, found := harness.SessionFrom(operationContext)
	if !found {
		return false, nil
	}
	value, operationError := tool.plugin.requestConfiguration(operationContext, session.InstanceID)
	if operationError != nil {
		return false, operationError
	}
	return value.Enabled && value.WorkerModel != "" && tool.plugin.unavailable == nil, nil
}

func (tool *instructionTool) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	session, found := harness.SessionFrom(operationContext)
	if !found {
		return atom.ToolResult{}, fmt.Errorf("instruction recovery has no session context")
	}
	available, operationError := tool.Available(operationContext)
	if operationError != nil {
		return atom.ToolResult{}, operationError
	}
	if !available {
		return atom.ToolResult{}, fmt.Errorf("instruction recovery is unavailable or has no configured worker model")
	}
	var input struct {
		Reason string `json:"reason"`
	}
	decoder := json.NewDecoder(bytes.NewReader(call.Input))
	decoder.DisallowUnknownFields()
	if operationError := decoder.Decode(&input); operationError != nil {
		return atom.ToolResult{}, operationError
	}
	if !utf8.ValidString(input.Reason) || strings.TrimSpace(input.Reason) == "" || len(input.Reason) > 4096 {
		return atom.ToolResult{}, fmt.Errorf("instruction recovery reason must contain 1 to 4096 UTF-8 bytes")
	}
	if operationError := tool.plugin.recoverInstructions(operationContext, session, call.ID, input.Reason); operationError != nil {
		return atom.ToolResult{}, operationError
	}
	return atom.ToolResult{CallID: call.ID, Status: atom.StatusOK, Content: []atom.Content{{Type: atom.Text, Text: `{"status":"ready"}`}}}, nil
}
