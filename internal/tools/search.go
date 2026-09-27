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

func NewSearch(reg *registry.Registry) *Search {
	return &Search{registry: reg}
}

func (s *Search) Name() string {
	return "search_tool"
}

func (s *Search) Description() string {
	return "Find tools by a category or text."
}

func (s *Search) Categories() []string {
	return []string{"system"}
}

func (s *Search) InputSchema() atom.Schema {
	return schema(`{"type":"object","properties":{"query":{"type":"string"},"category":{"type":"string"},"limit":{"type":"integer"}}}`)
}

func (s *Search) Check(ctx context.Context, call atom.ToolCall) atom.Verdict {
	return atom.Verdict{Kind: atom.VerdictAllow}
}

func (s *Search) Run(ctx context.Context, call atom.ToolCall) (atom.ToolResult, error) {
	var input struct {
		Query    string `json:"query"`
		Category string `json:"category"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal(call.Input, &input); err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: "search_tool: the input is not correct"}, err
	}
	found := s.registry.Find(input.Query, input.Category, input.Limit)
	specs := make([]atom.ToolSpec, 0, len(found))
	for _, item := range found {
		specs = append(specs, atom.ToolSpec{
			Name:        item.Name(),
			Description: item.Description(),
			Categories:  item.Categories(),
			InputSchema: item.InputSchema(),
		})
	}
	data, err := json.Marshal(specs)
	if err != nil {
		return atom.ToolResult{CallID: call.ID, Status: atom.StatusError, Error: err.Error()}, err
	}
	return atom.ToolResult{
		CallID:  call.ID,
		Status:  atom.StatusOK,
		Content: []atom.Content{{Type: atom.Text, Text: string(data)}},
	}, nil
}
