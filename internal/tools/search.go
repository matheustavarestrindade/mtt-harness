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
	return "Discover tools with deterministic text/category matching. Returns names, descriptions, categories, and input schemas. Matching tools are added to this session's tool group for subsequent model requests; search again when needed."
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
				"description": "Case-insensitive words to match against tool names, descriptions, and categories. Every word must appear. This is plain substring matching, not semantic search. Omit to search by category only; no query and no category returns no results.",
				"examples": ["file", "process output"]
			},
			"category": {
				"type": "string",
				"default": "",
				"description": "Optional exact category filter, ignoring case. When both category and query are set, tools must satisfy both. Omit or use an empty string for no category filter.",
				"examples": ["file", "process", "command", "agent", "system", "mcp"]
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
	found := searchTool.registry.Find(input.Query, input.Category, input.Limit)
	specifications := make([]atom.ToolSpec, 0, len(found))
	for _, item := range found {
		specifications = append(specifications, atom.ToolSpec{
			Name:        item.Name(),
			Description: item.Description(),
			Categories:  item.Categories(),
			InputSchema: item.InputSchema(),
		})
	}
	data, operationError := json.Marshal(specifications)
	if operationError != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: operationError.Error()}, operationError
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: string(data)}},
	}, nil
}
