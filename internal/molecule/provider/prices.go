package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (standardProvider *Standard) SetPrices(prices map[string]atom.Prices) {
	standardProvider.mutex.Lock()
	defer standardProvider.mutex.Unlock()
	standardProvider.inlinePrices = map[string]atom.Prices{}
	for identifier, price := range prices {
		standardProvider.inlinePrices[identifier] = price
	}
	for index := range standardProvider.models {
		if standardProvider.providerSpec.Billing == "subscription" {
			standardProvider.models[index].Prices = nil
			continue
		}
		if price, found := standardProvider.inlinePrices[standardProvider.models[index].ID]; found {
			value := price
			standardProvider.models[index].Prices = &value
		}
	}
}

func (standardProvider *Standard) loadConfiguredPrices(operationContext context.Context) (map[string]atom.Prices, error) {
	result := map[string]atom.Prices{}
	standardProvider.mutex.RLock()
	for identifier, price := range standardProvider.inlinePrices {
		result[identifier] = price
	}
	standardProvider.mutex.RUnlock()
	source := standardProvider.providerSpec.PriceTableURL
	if source == "" {
		return result, nil
	}
	data, operationError := standardProvider.readPriceTable(operationContext, source)
	if operationError != nil {
		return nil, operationError
	}
	var raw map[string]struct {
		Currency   string  `json:"currency"`
		Input      float64 `json:"input"`
		Output     float64 `json:"output"`
		CacheRead  float64 `json:"cache_read"`
		CacheWrite float64 `json:"cache_write"`
	}
	if operationError := json.Unmarshal(data, &raw); operationError != nil {
		return nil, operationError
	}
	for identifier, entry := range raw {
		currency := entry.Currency
		if currency == "" {
			currency = "USD"
		}
		result[identifier] = atom.Prices{
			Currency:   currency,
			Input:      entry.Input,
			Output:     entry.Output,
			CacheRead:  entry.CacheRead,
			CacheWrite: entry.CacheWrite,
		}
	}
	// Local prices are explicit overrides, including explicit zero prices.
	standardProvider.mutex.RLock()
	for identifier, price := range standardProvider.inlinePrices {
		result[identifier] = price
	}
	standardProvider.mutex.RUnlock()
	for identifier, price := range result {
		if operationError := validatePrices(price); operationError != nil {
			return nil, fmt.Errorf("price table model %q: %w", identifier, operationError)
		}
	}
	return result, nil
}

func (standardProvider *Standard) readPriceTable(operationContext context.Context, source string) ([]byte, error) {
	return standardProvider.readCatalogSource(operationContext, source, 1<<20)
}

func pricePerMillionTokens(value string) (float64, error) {
	if value == "" {
		return 0, nil
	}
	number, operationError := strconv.ParseFloat(value, 64)
	if operationError != nil {
		return 0, fmt.Errorf("invalid token price: %w", operationError)
	}
	number *= 1_000_000
	if number < 0 || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("token prices must be finite and non-negative")
	}
	return number, nil
}
