package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/internal/mcp"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
)

// The returned cleanup belongs to the application, not to this loader.
func loadMCP(operationContext context.Context, path string, toolRegistry *registry.Registry) (func() error, error) {
	servers, operationError := mcp.LoadFile(path)
	if errors.Is(operationError, os.ErrNotExist) {
		return func() error {
			return nil
		}, nil
	}
	if operationError != nil {
		return nil, operationError
	}
	var bindings []*mcpBinding
	closeAll := func() error {
		var failures []error
		for _, binding := range bindings {
			failures = append(failures, binding.client.Close())
			binding.remove()
		}
		return errors.Join(failures...)
	}
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		client, operationError := mcp.Start(operationContext, server, 30*time.Second)
		if operationError != nil {
			return nil, errors.Join(operationError, closeAll())
		}
		binding := &mcpBinding{client: client, server: server.Name, registry: toolRegistry}
		bindings = append(bindings, binding)
		if operationError := binding.refresh(operationContext); operationError != nil {
			return nil, errors.Join(operationError, closeAll())
		}
		client.OnToolsChanged(func(operationContext context.Context) {
			if operationError := binding.refresh(operationContext); operationError != nil {
				client.ReportError(fmt.Errorf("refresh MCP tools: %w", operationError))
			}
		})
	}
	return closeAll, nil
}

type mcpBinding struct {
	mutex    sync.Mutex
	client   *mcp.Client
	server   string
	registry *registry.Registry
	names    []string
}

func (binding *mcpBinding) refresh(operationContext context.Context) error {
	binding.mutex.Lock()
	defer binding.mutex.Unlock()
	tools, operationError := binding.client.ListTools(operationContext)
	if operationError != nil {
		return operationError
	}
	for _, name := range binding.names {
		binding.registry.Remove(name)
	}
	binding.names = nil
	for _, specification := range tools {
		tool := &mcp.Tool{Client: binding.client, Server: binding.server, Spec: specification}
		binding.registry.Upsert(tool)
		binding.names = append(binding.names, tool.Name())
	}
	return nil
}

func (binding *mcpBinding) remove() {
	binding.mutex.Lock()
	defer binding.mutex.Unlock()
	for _, name := range binding.names {
		binding.registry.Remove(name)
	}
	binding.names = nil
}
