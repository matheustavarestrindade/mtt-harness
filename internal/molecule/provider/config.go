package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Config struct {
	Spec          atom.ProviderSpec
	Prices        map[string]atom.Prices
	Models        []ModelConfiguration
	ModelDefaults ModelMetadata
}

type fileEntry struct {
	Name             string               `json:"name"`
	Protocol         string               `json:"protocol"`
	Authentication   string               `json:"authentication"`
	APIURL           string               `json:"api_url"`
	ModelListURL     string               `json:"model_list_url"`
	ModelListFormat  string               `json:"model_list_format"`
	PriceTableURL    string               `json:"price_table_url"`
	RefreshHours     int                  `json:"refresh_hours"`
	Prices           map[string]filePrice `json:"prices"`
	Models           []ModelConfiguration `json:"models"`
	ModelDefaults    ModelMetadata        `json:"model_defaults"`
	MetadataURL      string               `json:"metadata_url"`
	MetadataFormat   string               `json:"metadata_format"`
	MetadataProvider string               `json:"metadata_provider"`
	Billing          string               `json:"billing"`
}

type filePrice struct {
	Currency   string          `json:"currency"`
	Input      float64         `json:"input"`
	Output     float64         `json:"output"`
	CacheRead  float64         `json:"cache_read"`
	CacheWrite float64         `json:"cache_write"`
	Reasoning  *float64        `json:"reasoning"`
	Tiers      []filePriceTier `json:"tiers"`
}

type filePriceTier struct {
	AboveInputTokens int `json:"input_tokens_above"`
	catalogRates
}

type fileConfig struct {
	Providers []fileEntry `json:"providers"`
}

// LoadFile loads the complete provider catalog. It does not inject providers,
// endpoints, or model IDs that are absent from the selected JSON file.
func LoadFile(path string) ([]Config, error) {
	configurationJSON, operationError := os.ReadFile(path)
	if operationError != nil {
		return nil, operationError
	}
	var configurationFile fileConfig
	if operationError := json.Unmarshal(configurationJSON, &configurationFile); operationError != nil {
		return nil, operationError
	}
	var configurations []Config
	providerNames := map[string]bool{}
	for _, entry := range configurationFile.Providers {
		if operationError := validateProviderEntry(entry); operationError != nil {
			return nil, operationError
		}
		if providerNames[entry.Name] {
			return nil, fmt.Errorf("duplicate provider %q", entry.Name)
		}
		providerNames[entry.Name] = true
		if entry.RefreshHours == 0 {
			entry.RefreshHours = 24
		}
		prices := map[string]atom.Prices{}
		for modelID, price := range entry.Prices {
			currency := price.Currency
			if currency == "" {
				currency = "USD"
			}
			value := atom.Prices{Currency: currency, Input: price.Input, Output: price.Output, CacheRead: price.CacheRead, CacheWrite: price.CacheWrite, Reasoning: price.Reasoning, Source: path}
			for _, tier := range price.Tiers {
				value.Tiers = append(value.Tiers, atom.PriceTier{AboveInputTokens: tier.AboveInputTokens,
					Input: catalogRateOr(tier.Input, value.Input), Output: catalogRateOr(tier.Output, value.Output),
					CacheRead: catalogRateOr(tier.CacheRead, value.CacheRead), CacheWrite: catalogRateOr(tier.CacheWrite, value.CacheWrite), Reasoning: tier.Reasoning})
			}
			if operationError := validatePrices(value); operationError != nil {
				return nil, fmt.Errorf("provider %s model %s: %w", entry.Name, modelID, operationError)
			}
			prices[modelID] = value
		}
		configurations = append(configurations, Config{
			Spec: atom.ProviderSpec{Name: entry.Name, Protocol: entry.Protocol, Authentication: entry.Authentication,
				APIURL: entry.APIURL, ModelListURL: entry.ModelListURL, ModelListFormat: entry.ModelListFormat,
				PriceTableURL: entry.PriceTableURL, Interval: time.Duration(entry.RefreshHours) * time.Hour,
				MetadataURL: entry.MetadataURL, MetadataFormat: entry.MetadataFormat, MetadataProvider: entry.MetadataProvider, Billing: entry.Billing},
			Prices: prices, Models: entry.Models, ModelDefaults: entry.ModelDefaults,
		})
	}
	return configurations, nil
}

func validateProviderEntry(entry fileEntry) error {
	if entry.Name == "" || entry.APIURL == "" {
		return fmt.Errorf("provider name and api_url are required")
	}
	switch entry.Protocol {
	case "", "responses", "chat_completions":
	default:
		return fmt.Errorf("provider %s: protocol must be responses or chat_completions", entry.Name)
	}
	switch entry.Authentication {
	case "", "api_key", "none":
	case "chatgpt":
		if entry.Protocol != "responses" {
			return fmt.Errorf("ChatGPT authentication requires the responses protocol")
		}
	default:
		return fmt.Errorf("provider %s: authentication must be api_key, chatgpt or none", entry.Name)
	}
	switch entry.ModelListFormat {
	case "", "openai", "codex":
	default:
		return fmt.Errorf("provider %s: model_list_format must be openai or codex", entry.Name)
	}
	if entry.RefreshHours < 0 {
		return fmt.Errorf("provider refresh interval cannot be negative")
	}
	if entry.MetadataURL != "" && (entry.MetadataFormat != "models_dev" || entry.MetadataProvider == "") {
		return fmt.Errorf("provider %s: metadata_url requires metadata_format models_dev and metadata_provider", entry.Name)
	}
	if entry.MetadataURL == "" && (entry.MetadataFormat != "" || entry.MetadataProvider != "") {
		return fmt.Errorf("provider %s: metadata_url is required with metadata settings", entry.Name)
	}
	switch entry.Billing {
	case "", "tokens", "subscription":
	default:
		return fmt.Errorf("provider %s: billing must be tokens or subscription", entry.Name)
	}
	if operationError := validateModelMetadata(entry.ModelDefaults); operationError != nil {
		return operationError
	}
	modelIDs := map[string]bool{}
	for _, model := range entry.Models {
		if model.ID == "" || modelIDs[model.ID] {
			return fmt.Errorf("provider %s: model IDs must be nonempty and unique", entry.Name)
		}
		modelIDs[model.ID] = true
		if operationError := validateModelMetadata(model.ModelMetadata); operationError != nil {
			return operationError
		}
	}
	return nil
}

func validateModelMetadata(metadata ModelMetadata) error {
	if metadata.ContextMax != nil && *metadata.ContextMax < 0 {
		return fmt.Errorf("model context_max cannot be negative")
	}
	if metadata.Level != nil && *metadata.Level < 0 {
		return fmt.Errorf("model level cannot be negative")
	}
	seenEfforts := map[string]bool{}
	for _, effort := range metadata.ReasoningEfforts {
		if strings.TrimSpace(effort) == "" || effort != strings.TrimSpace(effort) || seenEfforts[effort] {
			return fmt.Errorf("reasoning_efforts must contain unique non-empty values")
		}
		seenEfforts[effort] = true
	}
	if metadata.DefaultReasoningEffort != nil && *metadata.DefaultReasoningEffort != "" && metadata.ReasoningEfforts != nil && !slices.Contains(metadata.ReasoningEfforts, *metadata.DefaultReasoningEffort) {
		return fmt.Errorf("default_reasoning_effort must be one of reasoning_efforts")
	}
	if metadata.ReasoningSummary != nil {
		switch *metadata.ReasoningSummary {
		case "", "auto", "concise", "detailed":
		default:
			return fmt.Errorf("reasoning_summary must be empty, auto, concise, or detailed")
		}
	}
	for _, mediaType := range append(append([]atom.MediaType{}, metadata.Input...), metadata.Output...) {
		switch mediaType {
		case atom.Text, atom.Image, atom.Audio, atom.Video, atom.File:
		default:
			return fmt.Errorf("unsupported model media type %q", mediaType)
		}
	}
	return nil
}
