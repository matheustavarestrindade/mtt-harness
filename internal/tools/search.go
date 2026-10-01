package tools

import (
	"context"
	"encoding/json"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
)

type Search struct {
	registry *registry.Registry
}

func NewSearch(toolRegistry *registry.Registry) *Search {
	return &Search{registry: toolRegistry}
}

func (searchTool *Search) Name() string {
	return "search_tool"
}

func (searchTool *Search) Description() string {
	return "Find tools by describing the operation you need. Ranks full tool usage and input specifications with the configured semantic or lexical search backend, or filters by category. Returns compact tool names and categories; found tools are enabled with instructions and schemas in the next model request."
}

func (searchTool *Search) Categories() []string {
	return []string{"system"}
}

func (searchTool *Search) InputSchema() atom.Schema {
	return schema(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"default": "",
				"description": "Describe the desired capability in plain text. Full tool documentation is ranked by the configured search backend; exact tool names are resolved directly, ignoring case. Omitted or empty means category-only search. No query and no category returns no results.",
				"examples": ["shell command exec", "file read", "process output"]
			},
			"category": {
				"type": "string",
				"default": "",
				"description": "Optional exact category filter, ignoring case and outer whitespace. Combined with query, restricts ranked results to this category. Omitted or empty means no category filter. Category-only results are sorted by tool name.",
				"examples": ["file", "process", "command", "shell", "agent", "system", "mcp"]
			},
			"limit": {
				"type": "integer",
				"default": 10,
				"description": "Maximum number of returned tools. Defaults to 10 when omitted or non-positive. Values above 50 are capped at 50."
			}
		}
	}`)
}

func (searchTool *Search) Check(operationContext context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (searchTool *Search) Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		Query    string `json:"query"`
		Category string `json:"category"`
		Limit    int    `json:"limit"`
	}
	if operationError := json.Unmarshal(call.Input, &input); operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "search_tool: the input is not correct"}, operationError
	}
	found, operationError := searchTool.registry.Find(operationContext, input.Query, input.Category, input.Limit)
	if operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
	}
	references := make([]atom.ToolReference, 0, len(found))
	for _, item := range found {
		references = append(references, atom.ToolReference{
			Name:       item.Name(),
			Categories: item.Categories(),
		})
	}
	data, operationError := json.Marshal(references)
	if operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: string(data)}},
	}, nil
}
