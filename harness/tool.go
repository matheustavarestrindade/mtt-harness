package harness

import (
	"context"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// Tool describes a callable capability. Failures return to the loop.
type Tool interface {
	Name() string
	Description() string
	Categories() []string
	InputSchema() atom.Schema
	Check(operationContext context.Context, call atom.ToolCall) atom.Verdict
	Run(operationContext context.Context, call atom.ToolCall) (atom.ToolResult, error)
}

// ToolSearchDocumentation optionally adds usage guidance to the embedding
// document. The tool's description and schema are always indexed. This extra
// documentation is never included in model tool definitions or search results.
type ToolSearchDocumentation interface {
	SearchDocument() string
}

// ScopedTool optionally limits a registered capability to the current request.
// Discovery and callable definitions use the same predicate as execution.
type ScopedTool interface {
	Available(context.Context) (bool, error)
}

func ToolAvailable(operationContext context.Context, tool Tool) (bool, error) {
	if scoped, supported := tool.(ScopedTool); supported {
		return scoped.Available(operationContext)
	}
	return true, nil
}

func (harnessRuntime *Harness) Tool(tool Tool) Unsubscribe {
	harnessRuntime.mutex.Lock()
	identifier := harnessRuntime.nextRegistrationID()
	name := tool.Name()
	harnessRuntime.tools[name] = registration[Tool]{identifier, tool}
	harnessRuntime.mutex.Unlock()
	return func() {
		harnessRuntime.mutex.Lock()
		defer harnessRuntime.mutex.Unlock()
		if harnessRuntime.tools[name].identifier == identifier {
			delete(harnessRuntime.tools, name)
		}
	}
}

func (harnessRuntime *Harness) RemoveTool(name string) {
	harnessRuntime.mutex.Lock()
	defer harnessRuntime.mutex.Unlock()
	delete(harnessRuntime.tools, name)
}

func (harnessRuntime *Harness) ToolByName(name string) (Tool, bool) {
	harnessRuntime.mutex.RLock()
	defer harnessRuntime.mutex.RUnlock()
	entry, found := harnessRuntime.tools[name]
	return entry.value, found
}

func (harnessRuntime *Harness) Tools() []Tool {
	harnessRuntime.mutex.RLock()
	defer harnessRuntime.mutex.RUnlock()
	tools := make([]Tool, 0, len(harnessRuntime.tools))
	for _, entry := range harnessRuntime.tools {
		tools = append(tools, entry.value)
	}
	sort.Slice(tools, func(first, second int) bool {
		return tools[first].Name() < tools[second].Name()
	})
	return tools
}
