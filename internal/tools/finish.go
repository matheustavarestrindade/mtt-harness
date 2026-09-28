package tools

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Finish struct{}

func (Finish) Name() string {
	return "finish"
}

func (Finish) Description() string {
	return "Complete the current child agent task and return the supplied result to the parent agent call. Available only to child agents; use it for the final deliverable. Other tool calls in the same model response finish before the child turn ends."
}

func (Finish) Categories() []string {
	return []string{"agent"}
}

func (Finish) InputSchema() atom.Schema {
	return schema(`{
		"type": "object",
		"properties": {
			"result": {
				"type": "string",
				"description": "Final deliverable sent to the parent agent call. Include the requested findings, relevant paths, and any unresolved limitations; the parent receives this text, not the child's entire conversation."
			}
		},
		"required": ["result"]
	}`)
}

func (Finish) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (Finish) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: "the agent stops"}},
	}, nil
}
