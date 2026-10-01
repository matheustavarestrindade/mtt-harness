package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

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
	return result, nil
}

func (standardProvider *Standard) readPriceTable(operationContext context.Context, source string) ([]byte, error) {
	if !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") {
		return os.ReadFile(source)
	}
	request, operationError := http.NewRequestWithContext(operationContext, http.MethodGet, source, nil)
	if operationError != nil {
		return nil, operationError
	}
	response, operationError := standardProvider.client.Do(request)
	if operationError != nil {
		return nil, operationError
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider: the price table gives the status %d", response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, 1<<20))
}

func pricePerMillionTokens(value string) float64 {
	if value == "" {
		return 0
	}
	var number float64
	if _, operationError := fmt.Sscanf(value, "%f", &number); operationError != nil {
		return 0
	}
	return number * 1_000_000
}
