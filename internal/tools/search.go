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
	return "Find tools by a category or text."
}

func (searchTool *Search) Categories() []string {
	return []string{"system"}
}

func (searchTool *Search) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"query":{"type":"string"},"category":{"type":"string"},"limit":{"type":"integer"}}}`)
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
