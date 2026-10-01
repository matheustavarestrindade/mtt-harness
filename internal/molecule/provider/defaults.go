package provider

import (
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

const CodexProvider = "openai-codex"

// Defaults supplies usable provider endpoints and conservative model metadata.
// Account access is checked by the provider when credentials are connected.
func Defaults() []Config {
	// DeepSeek's versioned names differ from its stable request IDs.
	// Capabilities: https://api-docs.deepseek.com/quick_start/pricing
	deepSeekFlashModel := codingModel("deepseek-flash", 1, 1000000, true)
	deepSeekFlashModel.Name = "DeepSeek-V4.1-Flash"
	deepSeekProModel := codingModel("deepseek-v4-pro", 2, 1000000, false)
	deepSeekProModel.Name = "DeepSeek-V4-Pro-0813"
	return []Config{
		{Spec: atom.ProviderSpec{Name: "openai", Protocol: "responses", Authentication: "api_key", APIURL: "https://api.openai.com/v1", ModelListURL: "https://api.openai.com/v1/models", Interval: 24 * time.Hour}, Models: []atom.ModelInfo{
			codingModel("gpt-6-astra", 3, 1050000, true),
			codingModel("gpt-6-sol", 2, 1050000, true),
			codingModel("gpt-6-luna", 1, 1050000, true),
			codingModel("gpt-5.5", 3, 272000, true),
			codingModel("gpt-5.4", 2, 272000, true),
			codingModel("gpt-5.4-mini", 1, 272000, true),
			codingModel("gpt-4o", 2, 128000, true),
			codingModel("gpt-4o-mini", 1, 128000, true),
		}},
		{Spec: atom.ProviderSpec{Name: "deepseek", Protocol: "responses", Authentication: "api_key", APIURL: "https://api.deepseek.com", ModelListURL: "https://api.deepseek.com/models", Interval: 24 * time.Hour}, Models: []atom.ModelInfo{
			deepSeekFlashModel,
			deepSeekProModel,
		}},
		{Spec: atom.ProviderSpec{Name: CodexProvider, Protocol: "responses", Authentication: "chatgpt", APIURL: "https://chatgpt.com/backend-api/codex"}, Models: []atom.ModelInfo{
			codingModel("gpt-6-sol", 2, 272000, true),
			codingModel("gpt-6-luna", 1, 272000, true),
			codingModel("gpt-5.5", 3, 272000, true),
		}},
	}
}

func codingModel(identifier string, level int, contextMax int, images bool) atom.ModelInfo {
	model := atom.ModelInfo{ID: identifier, Level: level, ContextMax: contextMax, Tools: true, Input: []atom.MediaType{atom.Text}, Output: []atom.MediaType{atom.Text}}
	if images {
		model.Input = append(model.Input, atom.Image)
	}
	return model
}

// WithDefaults preserves file overrides and supplements them with the built-in
// providers. Empty fields in an override inherit the built-in values.
func WithDefaults(configurations []Config) []Config {
	result := Defaults()
	for _, configuration := range configurations {
		position := -1
		for index := range result {
			if result[index].Spec.Name == configuration.Spec.Name {
				position = index
				break
			}
		}
		if position < 0 {
			result = append(result, configuration)
			continue
		}
		base := result[position]
		if configuration.Spec.APIURL != "" {
			base.Spec.APIURL = configuration.Spec.APIURL
		}
		if configuration.Spec.ModelListURL != "" {
			base.Spec.ModelListURL = configuration.Spec.ModelListURL
		}
		if configuration.Spec.PriceTableURL != "" {
			base.Spec.PriceTableURL = configuration.Spec.PriceTableURL
		}
		if configuration.Spec.Protocol != "" {
			base.Spec.Protocol = configuration.Spec.Protocol
		}
		if configuration.Spec.Authentication != "" {
			base.Spec.Authentication = configuration.Spec.Authentication
		}
		if configuration.Spec.Interval > 0 {
			base.Spec.Interval = configuration.Spec.Interval
		}
		base.Prices = configuration.Prices
		base.Models = MergeModels(base.Models, configuration.Models)
		result[position] = base
	}
	return result
}

// MergeModels replaces matching IDs while keeping metadata for other models.
func MergeModels(base []atom.ModelInfo, overrides []atom.ModelInfo) []atom.ModelInfo {
	result := append([]atom.ModelInfo(nil), base...)
	for _, model := range overrides {
		found := false
		for index := range result {
			if result[index].ID == model.ID {
				result[index] = model
				found = true
				break
			}
		}
		if !found {
			result = append(result, model)
		}
	}
	return result
}
