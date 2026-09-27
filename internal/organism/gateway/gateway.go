package gateway

import (
	"context"
	"fmt"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Gateway struct {
	mu        sync.RWMutex
	providers map[string]harness.Provider
	models    map[string]harness.Provider
}

func New() *Gateway {
	return &Gateway{
		providers: map[string]harness.Provider{},
		models:    map[string]harness.Provider{},
	}
}

func (g *Gateway) Add(provider harness.Provider) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.providers[provider.Name()]; exists {
		return fmt.Errorf("gateway: the provider %q is in the gateway", provider.Name())
	}
	g.providers[provider.Name()] = provider
	for _, model := range provider.Models() {
		g.models[model.ID] = provider
	}
	return nil
}

func (g *Gateway) Provider(name string) (harness.Provider, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	provider, ok := g.providers[name]
	return provider, ok
}

func (g *Gateway) Providers() []harness.Provider {
	g.mu.RLock()
	defer g.mu.RUnlock()
	list := make([]harness.Provider, 0, len(g.providers))
	for _, provider := range g.providers {
		list = append(list, provider)
	}
	return list
}

func (g *Gateway) Refresh(ctx context.Context, name string) ([]atom.ModelInfo, error) {
	provider, ok := g.Provider(name)
	if !ok {
		return nil, fmt.Errorf("gateway: the provider %q is not in the gateway", name)
	}
	refresher, ok := provider.(harness.Refresher)
	if !ok {
		return nil, fmt.Errorf("gateway: the provider %q cannot refresh the models", name)
	}
	models, err := refresher.Refresh(ctx)
	if err != nil {
		return nil, err
	}
	g.mu.Lock()
	for _, model := range models {
		g.models[model.ID] = provider
	}
	g.mu.Unlock()
	return models, nil
}

func (g *Gateway) Model(id string) (atom.ModelInfo, harness.Provider, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	provider, ok := g.models[id]
	if !ok {
		return atom.ModelInfo{}, nil, false
	}
	for _, model := range provider.Models() {
		if model.ID == id {
			return model, provider, true
		}
	}
	return atom.ModelInfo{}, nil, false
}
