package provider

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type catalogModel struct {
	ID string
	ModelMetadata
	Prices *atom.Prices
}

type openAIModelEntry struct {
	ID string `json:"id"`
	ModelMetadata
	Pricing *struct {
		Prompt          string `json:"prompt"`
		Completion      string `json:"completion"`
		InputCacheRead  string `json:"input_cache_read"`
		InputCacheWrite string `json:"input_cache_write"`
	} `json:"pricing"`
}

// The Codex wire fields are documented by openai/codex, rust-v0.101.0:
// codex-rs/protocol/src/openai_models.rs and codex-api/src/endpoint/models.rs.
// supports_parallel_tool_calls concerns parallelism, not basic tool support.
type codexModelEntry struct {
	Slug                     string           `json:"slug"`
	DisplayName              *string          `json:"display_name"`
	ContextWindow            *int             `json:"context_window"`
	InputModalities          []atom.MediaType `json:"input_modalities"`
	DefaultReasoningEffort   *string          `json:"default_reasoning_level"`
	SupportedReasoningLevels *[]struct {
		Effort string `json:"effort"`
	} `json:"supported_reasoning_levels"`
	SupportsReasoningSummaries *bool `json:"supports_reasoning_summaries"`
}

func decodeModelCatalog(responseBody io.Reader, format string) ([]catalogModel, error) {
	decoder := json.NewDecoder(io.LimitReader(responseBody, 16*1024*1024))
	var catalog []catalogModel
	switch format {
	case "", "openai":
		var response struct {
			Data *[]openAIModelEntry `json:"data"`
		}
		if operationError := decoder.Decode(&response); operationError != nil {
			return nil, fmt.Errorf("decode OpenAI-compatible model catalog: %w", operationError)
		}
		if response.Data == nil {
			return nil, fmt.Errorf("model catalog must contain a data array")
		}
		for _, entry := range *response.Data {
			model := catalogModel{ID: entry.ID, ModelMetadata: entry.ModelMetadata}
			if entry.Pricing != nil && entry.Pricing.Prompt != "" && entry.Pricing.Completion != "" {
				model.Prices = &atom.Prices{Currency: "USD", CacheReadUnknown: entry.Pricing.InputCacheRead == "", CacheWriteUnknown: entry.Pricing.InputCacheWrite == ""}
				for _, rate := range []struct {
					source string
					target *float64
				}{
					{entry.Pricing.Prompt, &model.Prices.Input}, {entry.Pricing.Completion, &model.Prices.Output},
					{entry.Pricing.InputCacheRead, &model.Prices.CacheRead}, {entry.Pricing.InputCacheWrite, &model.Prices.CacheWrite},
				} {
					value, operationError := pricePerMillionTokens(rate.source)
					if operationError != nil {
						return nil, fmt.Errorf("catalog model %q: %w", entry.ID, operationError)
					}
					*rate.target = value
				}
			}
			catalog = append(catalog, model)
		}
	case "codex":
		var response struct {
			Models *[]codexModelEntry `json:"models"`
		}
		if operationError := decoder.Decode(&response); operationError != nil {
			return nil, fmt.Errorf("decode Codex model catalog: %w", operationError)
		}
		if response.Models == nil {
			return nil, fmt.Errorf("model catalog must contain a models array")
		}
		for _, entry := range *response.Models {
			metadata := ModelMetadata{Name: entry.DisplayName, ContextMax: entry.ContextWindow, Input: entry.InputModalities, DefaultReasoningEffort: entry.DefaultReasoningEffort}
			if entry.SupportedReasoningLevels != nil {
				metadata.ReasoningEfforts = []string{}
				for _, level := range *entry.SupportedReasoningLevels {
					metadata.ReasoningEfforts = append(metadata.ReasoningEfforts, level.Effort)
				}
				supported := len(metadata.ReasoningEfforts) > 0
				metadata.Reasoning = &supported
			}
			if entry.SupportedReasoningLevels == nil && entry.DefaultReasoningEffort != nil && *entry.DefaultReasoningEffort != "" {
				supported := true
				metadata.Reasoning = &supported
				metadata.ReasoningEfforts = []string{*entry.DefaultReasoningEffort}
			}
			if entry.SupportsReasoningSummaries != nil {
				summary := ""
				if *entry.SupportsReasoningSummaries {
					summary = "auto"
				}
				metadata.ReasoningSummary = &summary
			}
			catalog = append(catalog, catalogModel{ID: entry.Slug, ModelMetadata: metadata})
		}
	default:
		return nil, fmt.Errorf("unsupported model catalog format %q", format)
	}
	if operationError := decoder.Decode(new(any)); operationError != io.EOF {
		return nil, fmt.Errorf("model catalog contains invalid trailing data")
	}
	modelIDs := map[string]bool{}
	for _, model := range catalog {
		if model.ID == "" || modelIDs[model.ID] {
			return nil, fmt.Errorf("model catalog IDs must be nonempty and unique")
		}
		modelIDs[model.ID] = true
		if operationError := validateModelMetadata(model.ModelMetadata); operationError != nil {
			return nil, operationError
		}
	}
	return catalog, nil
}
