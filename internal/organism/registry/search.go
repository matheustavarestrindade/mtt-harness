package registry

import (
	"context"
	"fmt"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/toolsearch"
)

// Find resolves exact names or ranks embedding documents. Categories are strict
// filters. Category-only discovery needs no embedding request.
func (toolRegistry *Registry) Find(operationContext context.Context, query string, category string, limit int) ([]harness.Tool, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	query, category = strings.ToLower(strings.TrimSpace(query)), strings.TrimSpace(category)
	if query == "" && category == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = defaultLimit
	}
	limit = min(limit, maximumLimit)
	var documents []toolsearch.Document
	candidates := map[string]harness.Tool{}
	var exact []harness.Tool
	for _, tool := range toolRegistry.All() {
		documents = append(documents, toolsearch.Describe(tool))
		if category != "" && !hasCategory(tool, category) {
			continue
		}
		candidates[tool.Name()] = tool
		if query == "" || strings.EqualFold(tool.Name(), query) {
			exact = append(exact, tool)
		}
	}
	if query == "" || len(exact) > 0 {
		return exact[:min(limit, len(exact))], nil
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	if toolRegistry.searcher == nil {
		return nil, fmt.Errorf("tool search backend is not configured")
	}
	matches, operationError := toolRegistry.searcher.Search(operationContext, documents, query)
	if operationError != nil {
		return nil, operationError
	}
	var found []harness.Tool
	for _, match := range matches {
		tool, permitted := candidates[match.ID]
		if !permitted {
			continue
		}
		found = append(found, tool)
		if len(found) == limit {
			break
		}
	}
	return found, nil
}

func hasCategory(tool harness.Tool, category string) bool {
	for _, toolCategory := range tool.Categories() {
		if strings.EqualFold(toolCategory, category) {
			return true
		}
	}
	return false
}
