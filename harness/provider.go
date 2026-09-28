package harness

import (
	"context"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// Provider serves a cached model catalog and cancellable response streams.
type Provider interface {
	Name() string
	Models() []atom.ModelInfo
	Stream(operationContext context.Context, request atom.Request) (Stream, error)
}

// Stream returns io.EOF after its last part and honors cancellation in Recv.
type Stream interface {
	Recv(operationContext context.Context) (atom.ResponsePart, error)
}
type Refresher interface {
	Refresh(operationContext context.Context) ([]atom.ModelInfo, error)
}

// TokenCounter lets adapters supply the provider's exact token accounting.
// Without it the context builder estimates text bytes and reserves media space.
type TokenCounter interface {
	CountTokens(operationContext context.Context, request atom.Request) (int, error)
}

func (harnessRuntime *Harness) Provider(provider Provider) Unsubscribe {
	harnessRuntime.mutex.Lock()
	identifier := harnessRuntime.nextRegistrationID()
	name := provider.Name()
	harnessRuntime.providers[name] = registration[Provider]{identifier, provider}
	harnessRuntime.mutex.Unlock()
	return func() {
		harnessRuntime.mutex.Lock()
		defer harnessRuntime.mutex.Unlock()
		if harnessRuntime.providers[name].identifier == identifier {
			delete(harnessRuntime.providers, name)
		}
	}
}

func (harnessRuntime *Harness) ProviderByName(name string) (Provider, bool) {
	harnessRuntime.mutex.RLock()
	defer harnessRuntime.mutex.RUnlock()
	entry, found := harnessRuntime.providers[name]
	return entry.value, found
}

func (harnessRuntime *Harness) Providers() []Provider {
	harnessRuntime.mutex.RLock()
	defer harnessRuntime.mutex.RUnlock()
	providers := make([]Provider, 0, len(harnessRuntime.providers))
	for _, entry := range harnessRuntime.providers {
		providers = append(providers, entry.value)
	}
	sort.Slice(providers, func(first, second int) bool {
		return providers[first].Name() < providers[second].Name()
	})
	return providers
}
