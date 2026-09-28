package registry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

const (
	defaultLimit = 10
	maximumLimit = 50
)

type Registry struct {
	harnessRuntime *harness.Harness
}

func New(harnessRuntime *harness.Harness) *Registry {
	return &Registry{harnessRuntime: harnessRuntime}
}

func (toolRegistry *Registry) Add(tool harness.Tool) error {
	if tool.Name() == "" || len(tool.Categories()) == 0 {
		return fmt.Errorf("registry: tool name and categories are required")
	}
	if _, exists := toolRegistry.harnessRuntime.ToolByName(tool.Name()); exists {
		return fmt.Errorf("registry: the tool %q is in the registry", tool.Name())
	}
	toolRegistry.harnessRuntime.Tool(tool)
	return nil
}

func (toolRegistry *Registry) Upsert(tool harness.Tool) {
	toolRegistry.harnessRuntime.Tool(tool)
}

func (toolRegistry *Registry) Remove(name string) {
	toolRegistry.harnessRuntime.RemoveTool(name)
}

func (toolRegistry *Registry) Get(name string) (harness.Tool, bool) {
	return toolRegistry.harnessRuntime.ToolByName(name)
}

func (toolRegistry *Registry) All() []harness.Tool {
	return toolRegistry.harnessRuntime.Tools()
}

type match struct {
	tool  harness.Tool
	group int
	name  string
}

func (toolRegistry *Registry) Find(query string, category string, limit int) []harness.Tool {
	words := strings.Fields(strings.ToLower(query))
	if category == "" && len(words) == 0 {
		return nil
	}
	var matches []match
	for _, tool := range toolRegistry.All() {
		name := strings.ToLower(tool.Name())
		description := strings.ToLower(tool.Description())
		group := 0
		if category != "" {
			for _, item := range tool.Categories() {
				if strings.EqualFold(item, category) {
					group = 1
					break
				}
			}
		}
		if category != "" && group == 0 {
			continue
		}
		if group == 0 && len(words) > 0 && containsAll(name, words) {
			group = 2
		}
		searchText := name + " " + description + " " + strings.ToLower(strings.Join(tool.Categories(), " "))
		if len(words) > 0 && !containsAll(searchText, words) {
			continue
		}
		if group == 0 && len(words) > 0 && containsAll(searchText, words) {
			group = 3
		}
		if group > 0 {
			matches = append(matches, match{tool: tool, group: group, name: name})
		}
	}
	sort.Slice(matches, func(index, otherIndex int) bool {
		if matches[index].group != matches[otherIndex].group {
			return matches[index].group < matches[otherIndex].group
		}
		return matches[index].name < matches[otherIndex].name
	})
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maximumLimit {
		limit = maximumLimit
	}
	if len(matches) > limit {
		matches = matches[:limit]
	}
	result := make([]harness.Tool, 0, len(matches))
	for _, item := range matches {
		result = append(result, item.tool)
	}
	return result
}

func containsAll(text string, words []string) bool {
	for _, word := range words {
		if !strings.Contains(text, word) {
			return false
		}
	}
	return true
}
