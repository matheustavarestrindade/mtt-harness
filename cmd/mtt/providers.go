package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/providerauth"
)

func loadProviders(operationContext context.Context, path string, modelGateway *gateway.Gateway, database store.Store, authentication *providerauth.Service) {
	configurations, operationError := provider.LoadFile(path)
	if errors.Is(operationError, os.ErrNotExist) {
		configurations = nil
		operationError = nil
	}
	requireStartupSuccess(operationError, "load provider configuration")
	for _, configuration := range provider.WithDefaults(configurations) {
		standardProvider := provider.New(configuration.Spec)
		standardProvider.SetPrices(configuration.Prices)
		standardProvider.SetModelConfiguration(configuration.Models)
		standardProvider.SetKeyResolver(func(operationContext context.Context, instanceID string, name string) (string, error) {
			return database.Secrets().ResolveKey(operationContext, instanceID, name)
		})
		if configuration.Spec.Authentication == "chatgpt" {
			standardProvider.SetHeaderResolver(authentication.Headers)
		}
		cachedModels, cacheError := database.Providers().Models(operationContext, configuration.Spec.Name)
		requireStartupSuccess(cacheError, "load provider model cache")
		cachedModels = provider.MergeModels(cachedModels, configuration.Models)
		if len(cachedModels) > 0 {
			standardProvider.SetModels(cachedModels)
		}
		requireStartupSuccess(modelGateway.Add(standardProvider), "register provider "+configuration.Spec.Name)
		requireStartupSuccess(database.Providers().Save(operationContext, configuration.Spec), "save provider "+configuration.Spec.Name)
		requireStartupSuccess(database.Providers().SaveModels(operationContext, configuration.Spec.Name, standardProvider.Models()), "save configured provider models")
		refresh(operationContext, modelGateway, database, configuration.Spec)
		log.Printf("mtt: the provider %s is in use", configuration.Spec.Name)
	}
}

func refresh(operationContext context.Context, modelGateway *gateway.Gateway, database store.Store, providerSpec atom.ProviderSpec) {
	if providerSpec.ModelListURL == "" {
		return
	}
	refreshedModels, operationError := modelGateway.Refresh(operationContext, providerSpec.Name)
	if operationError != nil {
		log.Printf("mtt: the provider refresh is not complete: %v", operationError)
	}
	if operationError == nil {
		if saveError := database.Providers().SaveModels(operationContext, providerSpec.Name, refreshedModels); saveError != nil {
			log.Printf("mtt: save model refresh: %v", saveError)
		}
		log.Printf("mtt: the provider %s gives %d models", providerSpec.Name, len(refreshedModels))
	}
	if providerSpec.Interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(providerSpec.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-operationContext.Done():
				return
			case <-ticker.C:
			}
			refreshedModels, operationError := modelGateway.Refresh(operationContext, providerSpec.Name)
			if operationError != nil {
				log.Printf("mtt: refresh provider: %v", operationError)
				continue
			}
			if operationError := database.Providers().SaveModels(operationContext, providerSpec.Name, refreshedModels); operationError != nil {
				log.Printf("mtt: save model refresh: %v", operationError)
			}
		}
	}()
}
