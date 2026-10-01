package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// Refresh replaces availability only after a complete successful catalog fetch.
// A failed/malformed response leaves the last usable catalog unchanged.
func (standardProvider *Standard) Refresh(operationContext context.Context) ([]atom.ModelInfo, error) {
	if standardProvider.providerSpec.ModelListURL == "" {
		return nil, fmt.Errorf("provider %s: model_list_url is not configured; the catalog is static", standardProvider.Name())
	}
	operationContext, cancelRefresh := context.WithTimeout(operationContext, 30*time.Second)
	defer cancelRefresh()
	prices, operationError := standardProvider.loadConfiguredPrices(operationContext)
	if operationError != nil {
		return nil, operationError
	}
	request, operationError := http.NewRequestWithContext(operationContext, http.MethodGet, standardProvider.providerSpec.ModelListURL, nil)
	if operationError != nil {
		return nil, operationError
	}
	if operationError := standardProvider.applyAuthenticationHeaders(request); operationError != nil {
		return nil, operationError
	}
	response, operationError := standardProvider.client.Do(request)
	if operationError != nil {
		return nil, operationError
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider %s: model list returned HTTP %d", standardProvider.Name(), response.StatusCode)
	}
	catalog, operationError := decodeModelCatalog(response.Body, standardProvider.providerSpec.ModelListFormat)
	if operationError != nil {
		return nil, operationError
	}
	cachedByID := map[string]atom.ModelInfo{}
	for _, model := range standardProvider.Models() {
		cachedByID[model.ID] = model
	}
	standardProvider.mutex.RLock()
	defaults := standardProvider.modelDefaults
	standardProvider.mutex.RUnlock()
	models := make([]atom.ModelInfo, 0, len(catalog))
	for _, catalogModel := range catalog {
		model, cached := cachedByID[catalogModel.ID]
		if !cached {
			model = newCatalogModel(catalogModel.ID)
		}
		model = applyModelMetadata(model, defaults)
		model = applyModelMetadata(model, catalogModel.ModelMetadata)
		model = standardProvider.configuredModelMetadata(model)
		model.Prices = catalogModel.Prices
		if price, configured := prices[model.ID]; configured {
			model.Prices = &price
		}
		models = append(models, model)
	}
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	standardProvider.SetModels(models)
	return standardProvider.Models(), nil
}
