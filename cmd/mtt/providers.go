package main

import (
	"context"
	"errors"
	"log"
	"net/http"
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
	for _, configuration := range configurations {
		standardProvider := provider.New(configuration.Spec)
		standardProvider.SetPrices(configuration.Prices)
		switch configuration.Spec.Authentication {
		case "chatgpt":
			standardProvider.SetHeaderResolver(func(operationContext context.Context) (http.Header, error) {
				return authentication.ProviderAuthenticationHeaders(operationContext, configuration.Spec.Name)
			})
		case "", "api_key":
			standardProvider.SetKeyResolver(database.Secrets().ResolveKey)
		case "none":
			// This provider explicitly permits unauthenticated requests.
		}
		cachedModels, cacheError := database.Providers().Models(operationContext, configuration.Spec.Name)
		requireStartupSuccess(cacheError, "load provider model cache")
		standardProvider.ConfigureModels(configuration.ModelDefaults, configuration.Models, cachedModels)
		requireStartupSuccess(modelGateway.Add(standardProvider), "register provider "+configuration.Spec.Name)
		requireStartupSuccess(database.Providers().Save(operationContext, configuration.Spec), "save provider "+configuration.Spec.Name)
		requireStartupSuccess(database.Providers().SaveModels(operationContext, configuration.Spec.Name, standardProvider.Models()), "save configured provider models")
		startProviderModelRefresh(operationContext, modelGateway, database, configuration.Spec)
		log.Printf("mtt: the provider %s is in use", configuration.Spec.Name)
	}
}

func startProviderModelRefresh(operationContext context.Context, modelGateway *gateway.Gateway, database store.Store, providerSpec atom.ProviderSpec) {
	if providerSpec.ModelListURL == "" {
		return
	}
	refreshAndSaveProviderModels(operationContext, modelGateway, database, providerSpec.Name)
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
			refreshAndSaveProviderModels(operationContext, modelGateway, database, providerSpec.Name)
		}
	}()
}

func refreshAndSaveProviderModels(operationContext context.Context, modelGateway *gateway.Gateway, database store.Store, providerName string) {
	models, operationError := modelGateway.Refresh(operationContext, providerName)
	if operationError != nil {
		log.Printf("mtt: refresh provider %s: %v", providerName, operationError)
		return
	}
	if operationError := database.Providers().SaveModels(operationContext, providerName, models); operationError != nil {
		log.Printf("mtt: save provider %s model catalog: %v", providerName, operationError)
		return
	}
	log.Printf("mtt: the provider %s gives %d models", providerName, len(models))
}
