package registry

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

const (
	defaultLimit = 10
	maximumLimit = 50
)

type Registry struct {
	mu    sync.RWMutex
	tools map[string]harness.Tool
}

func New() *Registry {
	return &Registry{tools: map[string]harness.Tool{}}
}

func (r *Registry) Add(tool harness.Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[tool.Name()]; exists {
		return fmt.Errorf("registry: the tool %q is in the registry", tool.Name())
	}
	r.tools[tool.Name()] = tool
	return nil
}

func (r *Registry) Get(name string) (harness.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) All() []harness.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]harness.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		list = append(list, tool)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name() < list[j].Name()
	})
	return list
}

type match struct {
	tool  harness.Tool
	group int
	name  string
}

func (r *Registry) Find(query string, category string, limit int) []harness.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	words := strings.Fields(strings.ToLower(query))
	if category == "" && len(words) == 0 {
		return nil
	}
	var matches []match
	for _, tool := range r.tools {
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
		if group == 0 && len(words) > 0 && containsAll(name, words) {
			group = 2
		}
		if group == 0 && len(words) > 0 && containsAll(description+" "+strings.Join(tool.Categories(), " "), words) {
			group = 3
		}
		if group > 0 {
			matches = append(matches, match{tool: tool, group: group, name: name})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].group != matches[j].group {
			return matches[i].group < matches[j].group
		}
		return matches[i].name < matches[j].name
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
