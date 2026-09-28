package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (standardProvider *Standard) Refresh(operationContext context.Context) ([]atom.ModelInfo, error) {
	operationContext, cancel := context.WithTimeout(operationContext, 30*time.Second)
	defer cancel()
	if standardProvider.providerSpec.ModelListURL == "" {
		return nil, errors.New("provider: the model list URL is not in the provider data")
	}
	prices, operationError := standardProvider.prices(operationContext)
	if operationError != nil {
		return nil, operationError
	}
	request, operationError := http.NewRequestWithContext(operationContext, http.MethodGet, standardProvider.providerSpec.ModelListURL, nil)
	if operationError != nil {
		return nil, operationError
	}
	key, operationError := standardProvider.secret(operationContext)
	if operationError != nil {
		return nil, operationError
	}
	if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	response, operationError := standardProvider.client.Do(request)
	if operationError != nil {
		return nil, operationError
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider: the model list gives the status %d", response.StatusCode)
	}
	var list struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing *struct {
				Prompt          string `json:"prompt"`
				Completion      string `json:"completion"`
				InputCacheRead  string `json:"input_cache_read"`
				InputCacheWrite string `json:"input_cache_write"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if operationError := json.NewDecoder(response.Body).Decode(&list); operationError != nil {
		return nil, operationError
	}
	previous := map[string]atom.ModelInfo{}
	for _, model := range standardProvider.Models() {
		previous[model.ID] = model
	}
	var models []atom.ModelInfo
	for _, item := range list.Data {
		if item.ID == "" {
			continue
		}
		model := atom.ModelInfo{
			ID:     item.ID,
			Input:  []atom.MediaType{atom.Text},
			Output: []atom.MediaType{atom.Text},
		}
		if old, found := previous[item.ID]; found {
			model = old
		}
		model.Prices = nil
		if item.Pricing != nil {
			model.Prices = &atom.Prices{
				Currency:   "USD",
				Input:      perMillion(item.Pricing.Prompt),
				Output:     perMillion(item.Pricing.Completion),
				CacheRead:  perMillion(item.Pricing.InputCacheRead),
				CacheWrite: perMillion(item.Pricing.InputCacheWrite),
			}
		}
		if price, found := prices[item.ID]; found {
			model.Prices = &price
		}
		models = append(models, model)
	}
	standardProvider.SetModels(models)
	return standardProvider.Models(), nil
}
