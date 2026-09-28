package gateway

import (
	"context"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// Gateway resolves models directly from the authoritative plugin catalog.
// Qualified IDs use provider/model; bare IDs work only when unambiguous.
type Gateway struct{ harnessRuntime *harness.Harness }

func New(harnessRuntime *harness.Harness) *Gateway {
	return &Gateway{harnessRuntime: harnessRuntime}
}

func (modelGateway *Gateway) Add(provider harness.Provider) error {
	if provider.Name() == "" {
		return fmt.Errorf("provider name is required")
	}
	if _, found := modelGateway.Provider(provider.Name()); found {
		return fmt.Errorf("provider %q is already registered", provider.Name())
	}
	modelGateway.harnessRuntime.Provider(provider)
	return nil
}

func (modelGateway *Gateway) Provider(name string) (harness.Provider, bool) {
	return modelGateway.harnessRuntime.ProviderByName(name)
}
func (modelGateway *Gateway) Providers() []harness.Provider {
	return modelGateway.harnessRuntime.Providers()
}

func (modelGateway *Gateway) Refresh(operationContext context.Context, name string) ([]atom.ModelInfo, error) {
	provider, found := modelGateway.Provider(name)
	if !found {
		return nil, fmt.Errorf("provider %q is not registered", name)
	}
	refresher, supported := provider.(harness.Refresher)
	if !supported {
		return nil, fmt.Errorf("provider %q does not support refresh", name)
	}
	return refresher.Refresh(operationContext)
}

func (modelGateway *Gateway) Resolve(identifier string) (atom.ModelInfo, harness.Provider, error) {
	for _, provider := range modelGateway.Providers() {
		for _, model := range provider.Models() {
			if identifier == provider.Name()+"/"+model.ID {
				return model, provider, nil
			}
		}
	}
	var selected atom.ModelInfo
	var selectedProvider harness.Provider
	for _, provider := range modelGateway.Providers() {
		for _, model := range provider.Models() {
			if identifier != model.ID {
				continue
			}
			if selectedProvider != nil {
				return atom.ModelInfo{}, nil, fmt.Errorf("model %q is ambiguous; use provider/model", identifier)
			}
			selected, selectedProvider = model, provider
		}
	}
	if selectedProvider == nil {
		return atom.ModelInfo{}, nil, fmt.Errorf("model %q is not registered", identifier)
	}
	return selected, selectedProvider, nil
}

func (modelGateway *Gateway) Model(identifier string) (atom.ModelInfo, harness.Provider, bool) {
	model, provider, operationError := modelGateway.Resolve(identifier)
	return model, provider, operationError == nil
}

func (modelGateway *Gateway) ResolveAllowed(identifier string, allowed []string) (atom.ModelInfo, harness.Provider, error) {
	model, provider, operationError := modelGateway.Resolve(identifier)
	if operationError != nil {
		return atom.ModelInfo{}, nil, operationError
	}
	if len(allowed) == 0 {
		return model, provider, nil
	}
	for _, candidate := range allowed {
		allowedModel, allowedProvider, operationError := modelGateway.Resolve(candidate)
		if operationError == nil && allowedModel.ID == model.ID && allowedProvider.Name() == provider.Name() {
			return model, provider, nil
		}
	}
	return atom.ModelInfo{}, nil, fmt.Errorf("model %q is not allowed for this instance", identifier)
}
