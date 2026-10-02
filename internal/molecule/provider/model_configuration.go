package provider

import "github.com/matheustavarestrindade/mtt-harness/atom"

// ModelMetadata preserves omitted fields separately from explicit false/zero
// overrides. Catalog availability always comes from the remote list when used.
type ModelMetadata struct {
	Name                   *string          `json:"name,omitempty"`
	Level                  *int             `json:"level,omitempty"`
	Input                  []atom.MediaType `json:"input,omitempty"`
	Output                 []atom.MediaType `json:"output,omitempty"`
	Tools                  *bool            `json:"tools,omitempty"`
	ContextMax             *int             `json:"context_max,omitempty"`
	Reasoning              *bool            `json:"reasoning,omitempty"`
	ReasoningEfforts       []string         `json:"reasoning_efforts,omitempty"`
	DefaultReasoningEffort *string          `json:"default_reasoning_effort,omitempty"`
	ReasoningSummary       *string          `json:"reasoning_summary,omitempty"`
}

type ModelConfiguration struct {
	ID string `json:"id"`
	ModelMetadata
}

func applyModelMetadata(model atom.ModelInfo, metadata ModelMetadata) atom.ModelInfo {
	if metadata.Name != nil {
		model.Name = *metadata.Name
	}
	if metadata.Level != nil {
		model.Level = *metadata.Level
	}
	if metadata.Input != nil {
		model.Input = append([]atom.MediaType{}, metadata.Input...)
	}
	if metadata.Output != nil {
		model.Output = append([]atom.MediaType{}, metadata.Output...)
	}
	if metadata.Tools != nil {
		model.Tools = *metadata.Tools
		model.ToolSupportUnknown = false
	}
	if metadata.ContextMax != nil {
		model.ContextMax = *metadata.ContextMax
	}
	if metadata.Reasoning != nil {
		model.Reasoning = *metadata.Reasoning
		if !model.Reasoning {
			model.ReasoningEfforts = nil
			model.DefaultReasoningEffort = ""
		}
	}
	if metadata.ReasoningEfforts != nil {
		model.ReasoningEfforts = append([]string{}, metadata.ReasoningEfforts...)
	}
	if metadata.DefaultReasoningEffort != nil {
		model.DefaultReasoningEffort = *metadata.DefaultReasoningEffort
	}
	if metadata.ReasoningSummary != nil {
		model.ReasoningSummary = *metadata.ReasoningSummary
	}
	return model
}

// ConfigureModels applies file metadata to cached models without adding IDs to a
// remote catalog. Static providers explicitly use their configured model IDs.
func (standardProvider *Standard) ConfigureModels(defaults ModelMetadata, configurations []ModelConfiguration, cachedModels []atom.ModelInfo) {
	standardProvider.mutex.Lock()
	standardProvider.modelDefaults = defaults
	standardProvider.configuredModels = append([]ModelConfiguration(nil), configurations...)
	standardProvider.mutex.Unlock()
	if standardProvider.providerSpec.ModelListURL == "" {
		cachedModels = nil
		for _, configuration := range configurations {
			cachedModels = append(cachedModels, newCatalogModel(configuration.ID))
		}
	}
	for modelIndex := range cachedModels {
		cachedModels[modelIndex] = standardProvider.configuredModelMetadata(applyModelMetadata(cachedModels[modelIndex], defaults))
	}
	standardProvider.SetModels(cachedModels)
}

func newCatalogModel(modelID string) atom.ModelInfo {
	return atom.ModelInfo{ID: modelID, Input: []atom.MediaType{atom.Text}, Output: []atom.MediaType{atom.Text}, ToolSupportUnknown: true}
}

func (standardProvider *Standard) configuredModelMetadata(model atom.ModelInfo) atom.ModelInfo {
	standardProvider.mutex.RLock()
	defer standardProvider.mutex.RUnlock()
	for _, configuration := range standardProvider.configuredModels {
		if configuration.ID == model.ID {
			return applyModelMetadata(model, configuration.ModelMetadata)
		}
	}
	return model
}
