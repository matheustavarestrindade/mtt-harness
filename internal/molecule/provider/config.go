package provider

import (
	"encoding/json"
	"fmt"
	"os"
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
	Name            string               `json:"name"`
	Protocol        string               `json:"protocol"`
	Authentication  string               `json:"authentication"`
	APIURL          string               `json:"api_url"`
	ModelListURL    string               `json:"model_list_url"`
	ModelListFormat string               `json:"model_list_format"`
	PriceTableURL   string               `json:"price_table_url"`
	RefreshHours    int                  `json:"refresh_hours"`
	Prices          map[string]filePrice `json:"prices"`
	Models          []ModelConfiguration `json:"models"`
	ModelDefaults   ModelMetadata        `json:"model_defaults"`
}

type filePrice struct {
	Currency   string  `json:"currency"`
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
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
			prices[modelID] = atom.Prices{Currency: currency, Input: price.Input, Output: price.Output, CacheRead: price.CacheRead, CacheWrite: price.CacheWrite}
		}
		configurations = append(configurations, Config{
			Spec: atom.ProviderSpec{Name: entry.Name, Protocol: entry.Protocol, Authentication: entry.Authentication,
				APIURL: entry.APIURL, ModelListURL: entry.ModelListURL, ModelListFormat: entry.ModelListFormat,
				PriceTableURL: entry.PriceTableURL, Interval: time.Duration(entry.RefreshHours) * time.Hour},
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
	for _, mediaType := range append(append([]atom.MediaType{}, metadata.Input...), metadata.Output...) {
		switch mediaType {
		case atom.Text, atom.Image, atom.Audio, atom.File:
		default:
			return fmt.Errorf("unsupported model media type %q", mediaType)
		}
	}
	return nil
}
