package registry

import (
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/toolsearch"
)

const (
	defaultLimit = 10
	maximumLimit = 50
)

type Registry struct {
	harnessRuntime *harness.Harness
	searcher       toolsearch.Searcher
}

func New(harnessRuntime *harness.Harness, searcher toolsearch.Searcher) *Registry {
	return &Registry{harnessRuntime: harnessRuntime, searcher: searcher}
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
