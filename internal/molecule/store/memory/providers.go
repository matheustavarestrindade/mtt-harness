package memory

import (
	"context"
	"errors"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type providers struct{ store *Store }

func (providerStore *providers) Save(operationContext context.Context, providerSpec atom.ProviderSpec) error {
	providerStore.store.mutex.Lock()
	defer providerStore.store.mutex.Unlock()
	providerStore.store.providers[providerSpec.Name] = providerSpec
	return nil
}

func (providerStore *providers) Get(operationContext context.Context, identifier string) (atom.ProviderSpec, error) {
	providerStore.store.mutex.RLock()
	defer providerStore.store.mutex.RUnlock()
	providerSpec, found := providerStore.store.providers[identifier]
	if !found {
		return atom.ProviderSpec{}, errors.New("memory: the provider is not in the store")
	}
	return providerSpec, nil
}

func (providerStore *providers) All(operationContext context.Context) ([]atom.ProviderSpec, error) {
	providerStore.store.mutex.RLock()
	defer providerStore.store.mutex.RUnlock()
	list := make([]atom.ProviderSpec, 0, len(providerStore.store.providers))
	for _, providerSpec := range providerStore.store.providers {
		list = append(list, providerSpec)
	}
	sort.Slice(list, func(firstIndex, secondIndex int) bool {
		return list[firstIndex].Name < list[secondIndex].Name
	})
	return list, nil
}

func (providerStore *providers) Delete(operationContext context.Context, identifier string) error {
	providerStore.store.mutex.Lock()
	defer providerStore.store.mutex.Unlock()
	delete(providerStore.store.providers, identifier)
	return nil
}

func (providerStore *providers) SaveModels(operationContext context.Context, provider string, models []atom.ModelInfo) error {
	providerStore.store.mutex.Lock()
	defer providerStore.store.mutex.Unlock()
	providerStore.store.models[provider] = append([]atom.ModelInfo(nil), models...)
	return nil
}

func (providerStore *providers) Models(operationContext context.Context, provider string) ([]atom.ModelInfo, error) {
	providerStore.store.mutex.RLock()
	defer providerStore.store.mutex.RUnlock()
	return append([]atom.ModelInfo(nil), providerStore.store.models[provider]...), nil
}
