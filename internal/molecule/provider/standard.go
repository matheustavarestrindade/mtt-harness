package provider

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type KeyResolver func(operationContext context.Context, instanceID string, provider string) (string, error)

type Standard struct {
	providerSpec     atom.ProviderSpec
	client           *http.Client
	mutex            sync.RWMutex
	models           []atom.ModelInfo
	configuredModels []ModelConfiguration
	modelDefaults    ModelMetadata
	inlinePrices     map[string]atom.Prices
	key              KeyResolver
	headers          func(context.Context) (http.Header, error)
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

// Spec reports public transport metadata, never credentials.
func (standardProvider *Standard) Spec() atom.ProviderSpec {
	return standardProvider.providerSpec
}

// SetHeaderResolver supplies request-time authentication for account-based
// providers. It must be configured before the provider is published.
func (standardProvider *Standard) SetHeaderResolver(resolver func(context.Context) (http.Header, error)) {
	standardProvider.headers = resolver
}

func (standardProvider *Standard) applyAuthenticationHeaders(request *http.Request) error {
	if standardProvider.headers != nil {
		headers, operationError := standardProvider.headers(request.Context())
		if operationError != nil {
			return operationError
		}
		for name, values := range headers {
			request.Header[name] = append([]string(nil), values...)
		}
		return nil
	}
	key, operationError := standardProvider.resolveAPIKey(request.Context())
	if operationError != nil {
		return operationError
	}
	if key == "" && standardProvider.providerSpec.Authentication == "api_key" {
		return fmt.Errorf("connect %s with an API key in Providers", standardProvider.Name())
	}
	if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	return nil
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

func (standardProvider *Standard) resolveAPIKey(operationContext context.Context) (string, error) {
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
