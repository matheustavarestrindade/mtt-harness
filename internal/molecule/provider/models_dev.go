package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type modelsDevEntry struct {
	ID               string  `json:"id"`
	Name             *string `json:"name"`
	Tools            *bool   `json:"tool_call"`
	Reasoning        *bool   `json:"reasoning"`
	ReasoningOptions []struct {
		Type   string   `json:"type"`
		Values []string `json:"values"`
	} `json:"reasoning_options"`
	Limit struct {
		Context *int `json:"context"`
	} `json:"limit"`
	Modalities struct {
		Input  []string `json:"input"`
		Output []string `json:"output"`
	} `json:"modalities"`
	Cost *catalogPrices `json:"cost"`
}

type catalogRates struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
	Reasoning  *float64 `json:"reasoning"`
}

type catalogPrices struct {
	catalogRates
	Tiers []struct {
		catalogRates
		Tier struct {
			Type string `json:"type"`
			Size int    `json:"size"`
		} `json:"tier"`
	} `json:"tiers"`
}

func (standardProvider *Standard) loadModelMetadata(operationContext context.Context) (map[string]catalogModel, error) {
	specification := standardProvider.providerSpec
	if specification.MetadataURL == "" {
		return nil, nil
	}
	data, operationError := standardProvider.readCatalogSource(operationContext, specification.MetadataURL, metadataCatalogLimit)
	if operationError != nil {
		return nil, fmt.Errorf("load model metadata: %w", operationError)
	}
	switch specification.MetadataFormat {
	case "models_dev":
		return decodeModelsDevMetadata(data, specification.MetadataProvider, specification.MetadataURL, specification.Protocol == "responses")
	default:
		return nil, fmt.Errorf("unsupported metadata format %q", specification.MetadataFormat)
	}
}

func decodeModelsDevMetadata(data []byte, providerID, source string, responsesProtocol bool) (map[string]catalogModel, error) {
	var providers map[string]json.RawMessage
	if operationError := json.Unmarshal(data, &providers); operationError != nil {
		return nil, fmt.Errorf("decode metadata catalog: %w", operationError)
	}
	raw, found := providers[providerID]
	if !found {
		return nil, fmt.Errorf("metadata catalog has no provider %q", providerID)
	}
	var providerEntry struct {
		Models map[string]modelsDevEntry `json:"models"`
	}
	if operationError := json.Unmarshal(raw, &providerEntry); operationError != nil {
		return nil, fmt.Errorf("decode metadata provider %q: %w", providerID, operationError)
	}
	if providerEntry.Models == nil {
		return nil, fmt.Errorf("metadata provider %q has no models object", providerID)
	}
	models := make(map[string]catalogModel, len(providerEntry.Models))
	for identifier, entry := range providerEntry.Models {
		if identifier == "" || entry.ID != "" && entry.ID != identifier {
			return nil, fmt.Errorf("metadata model ID does not match its key %q", identifier)
		}
		metadata := ModelMetadata{Name: entry.Name, Tools: entry.Tools, ContextMax: entry.Limit.Context, Reasoning: entry.Reasoning,
			Input: catalogMediaTypes(entry.Modalities.Input), Output: catalogMediaTypes(entry.Modalities.Output)}
		if entry.ReasoningOptions != nil {
			metadata.ReasoningEfforts = []string{}
			for _, option := range entry.ReasoningOptions {
				// Responses defines effort=none as its thinking toggle. A chat
				// provider's separate toggle does not imply reasoning_effort=none.
				if responsesProtocol && option.Type == "toggle" && !slices.Contains(metadata.ReasoningEfforts, "none") {
					metadata.ReasoningEfforts = append(metadata.ReasoningEfforts, "none")
				}
				if option.Type == "effort" {
					for _, effort := range option.Values {
						if !slices.Contains(metadata.ReasoningEfforts, effort) {
							metadata.ReasoningEfforts = append(metadata.ReasoningEfforts, effort)
						}
					}
				}
			}
		}
		if operationError := validateModelMetadata(metadata); operationError != nil {
			return nil, fmt.Errorf("metadata model %q: %w", identifier, operationError)
		}
		prices, operationError := decodeCatalogPrices(entry.Cost, source)
		if operationError != nil {
			return nil, fmt.Errorf("metadata prices for %q: %w", identifier, operationError)
		}
		models[identifier] = catalogModel{ID: identifier, ModelMetadata: metadata, Prices: prices}
	}
	return models, nil
}

func catalogMediaTypes(values []string) []atom.MediaType {
	if values == nil {
		return nil
	}
	result := []atom.MediaType{}
	for _, value := range values {
		mediaType := atom.MediaType(value)
		if value == "pdf" {
			mediaType = atom.File
		}
		switch mediaType {
		case atom.Text, atom.Image, atom.Audio, atom.Video, atom.File:
			if !slices.Contains(result, mediaType) {
				result = append(result, mediaType)
			}
		}
	}
	return result
}

func decodeCatalogPrices(source *catalogPrices, origin string) (*atom.Prices, error) {
	if source == nil || source.Input == nil || source.Output == nil {
		return nil, nil // Missing rates are unknown, not free.
	}
	if operationError := validateCatalogRates(source.catalogRates); operationError != nil {
		return nil, operationError
	}
	prices := &atom.Prices{Currency: "USD", Input: *source.Input, Output: *source.Output,
		CacheRead: catalogRateOr(source.CacheRead, 0), CacheWrite: catalogRateOr(source.CacheWrite, 0), Reasoning: source.Reasoning, Source: origin,
		CacheReadUnknown: source.CacheRead == nil, CacheWriteUnknown: source.CacheWrite == nil}
	for _, entry := range source.Tiers {
		if entry.Tier.Type != "context" {
			return nil, nil // An unsupported billing rule must not become a base-rate estimate.
		}
		if entry.Tier.Size < 0 {
			return nil, fmt.Errorf("context price threshold cannot be negative")
		}
		if operationError := validateCatalogRates(entry.catalogRates); operationError != nil {
			return nil, operationError
		}
		tier := atom.PriceTier{AboveInputTokens: entry.Tier.Size, Input: catalogRateOr(entry.Input, prices.Input), Output: catalogRateOr(entry.Output, prices.Output),
			CacheRead: catalogRateOr(entry.CacheRead, prices.CacheRead), CacheWrite: catalogRateOr(entry.CacheWrite, prices.CacheWrite), Reasoning: entry.Reasoning,
			CacheReadUnknown: entry.CacheRead == nil && prices.CacheReadUnknown, CacheWriteUnknown: entry.CacheWrite == nil && prices.CacheWriteUnknown}
		prices.Tiers = append(prices.Tiers, tier)
	}
	return prices, nil
}

func catalogRateOr(value *float64, fallback float64) float64 {
	if value != nil {
		return *value
	}
	return fallback
}

func validateCatalogRates(rates catalogRates) error {
	for _, value := range []*float64{rates.Input, rates.Output, rates.CacheRead, rates.CacheWrite, rates.Reasoning} {
		if value != nil && (*value < 0 || math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return fmt.Errorf("token prices must be finite and non-negative")
		}
	}
	return nil
}
