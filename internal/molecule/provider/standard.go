package provider

import (
	"context"
	"net/http"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type KeyResolver func(operationContext context.Context, instanceID string, provider string) (string, error)

type Standard struct {
	providerSpec atom.ProviderSpec
	client       *http.Client
	mutex        sync.RWMutex
	models       []atom.ModelInfo
	inlinePrices map[string]atom.Prices
	key          KeyResolver
}

func New(providerSpec atom.ProviderSpec) *Standard {
	return &Standard{
		providerSpec: providerSpec,
		client:       &http.Client{Timeout: 0},
	}
}

func (standardProvider *Standard) Name() string {
	return standardProvider.providerSpec.Name
}

func (standardProvider *Standard) Models() []atom.ModelInfo {
	standardProvider.mutex.RLock()
	defer standardProvider.mutex.RUnlock()
	return append([]atom.ModelInfo(nil), standardProvider.models...)
}

func (standardProvider *Standard) SetModels(models []atom.ModelInfo) {
	standardProvider.mutex.Lock()
	defer standardProvider.mutex.Unlock()
	standardProvider.models = append([]atom.ModelInfo(nil), models...)
	for index := range standardProvider.models {
		if price, found := standardProvider.inlinePrices[standardProvider.models[index].ID]; found {
			priceCopy := price
			standardProvider.models[index].Prices = &priceCopy
		}
	}
}

func (standardProvider *Standard) SetKeyResolver(resolver KeyResolver) {
	standardProvider.mutex.Lock()
	defer standardProvider.mutex.Unlock()
	standardProvider.key = resolver
}

func (standardProvider *Standard) secret(operationContext context.Context) (string, error) {
	standardProvider.mutex.RLock()
	resolver := standardProvider.key
	standardProvider.mutex.RUnlock()
	if resolver == nil {
		return "", nil
	}
	instanceID := ""
	if session, found := harness.SessionFrom(operationContext); found {
		instanceID = session.InstanceID
	}
	return resolver(operationContext, instanceID, standardProvider.providerSpec.Name)
}
