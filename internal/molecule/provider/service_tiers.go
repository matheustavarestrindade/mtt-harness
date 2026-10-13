package provider

import (
	"fmt"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// Presets inherit the resolved base metadata. Only tiers advertised by the
// current catalog create selections; they disappear with their base or tier.
func (standardProvider *Standard) catalogServiceTierPresets(model atom.ModelInfo, tiers []catalogServiceTier) ([]atom.ModelInfo, error) {
	presets := make([]atom.ModelInfo, 0, len(tiers))
	seen := map[string]bool{}
	for _, tier := range tiers {
		if strings.TrimSpace(tier.ID) == "" || strings.TrimSpace(tier.ID) != tier.ID || strings.ContainsAny(tier.ID, "/@\r\n\t") || seen[tier.ID] {
			return nil, fmt.Errorf("catalog model %q has invalid or duplicate service tier %q", model.ID, tier.ID)
		}
		seen[tier.ID] = true
		preset := model
		preset.ID = model.ID + "@" + tier.ID
		preset.APIModel = model.ID
		preset.ServiceTier = tier.ID
		// Tier prices are independent. Do not estimate a priority request with
		// base-model token rates; explicit preset prices are applied by Refresh.
		preset.Prices = nil
		name := model.Name
		if name == "" {
			name = model.ID
		}
		tierName := tier.Name
		if strings.TrimSpace(tierName) == "" {
			tierName = tier.ID
		}
		preset.Name = name + " (" + tierName + ")"
		preset = standardProvider.configuredModelMetadata(preset)
		if operationError := preset.ValidateReasoningEffort(preset.DefaultReasoningEffort); operationError != nil {
			return nil, operationError
		}
		presets = append(presets, preset)
	}
	return presets, nil
}

func validateCatalogSelectionIDs(models []atom.ModelInfo) error {
	seen := map[string]bool{}
	for _, model := range models {
		if seen[model.ID] {
			return fmt.Errorf("model catalog selection ID %q is not unique", model.ID)
		}
		seen[model.ID] = true
	}
	return nil
}

// Keep the selected ID intact for metadata validation and accounting. A tier
// preset fixes its wire tier so request middleware cannot silently select another.
func (standardProvider *Standard) applyModelPreset(payload map[string]any, request atom.Request) error {
	for _, model := range standardProvider.Models() {
		if model.ID != request.Model || model.APIModel == "" {
			continue
		}
		if tier, explicit := request.Params["service_tier"]; explicit && tier != model.ServiceTier {
			return fmt.Errorf("model preset %q requires service tier %q", model.ID, model.ServiceTier)
		}
		payload["model"] = model.APIModel
		payload["service_tier"] = model.ServiceTier
		return nil
	}
	return nil
}
