package provider

import (
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (standardProvider *Standard) applyReasoningParameters(payload map[string]any, request atom.Request) error {
	modelInfo := atom.ModelInfo{ID: request.Model}
	for _, model := range standardProvider.Models() {
		if model.ID == request.Model {
			modelInfo = model
			break
		}
	}
	var parameterEffort any
	reasoning := map[string]any{}
	if standardProvider.providerSpec.Protocol == "responses" {
		if supplied, found := payload["reasoning"]; found {
			values, correct := supplied.(map[string]any)
			if !correct {
				return fmt.Errorf("reasoning must be an object")
			}
			for name, value := range values {
				reasoning[name] = value
			}
		}
		parameterEffort = reasoning["effort"]
	} else {
		parameterEffort = payload["reasoning_effort"]
	}
	effort := request.ReasoningEffort
	if parameterEffort != nil {
		value, correct := parameterEffort.(string)
		if !correct || value == "" {
			return fmt.Errorf("reasoning effort parameter must be a non-empty string")
		}
		if effort != "" && effort != value {
			return fmt.Errorf("conflicting typed and provider reasoning effort parameters")
		}
		effort = value
	}
	if effort == "" {
		effort = modelInfo.DefaultReasoningEffort
	}
	if operationError := modelInfo.ValidateReasoningEffort(effort); operationError != nil {
		return operationError
	}
	if standardProvider.providerSpec.Protocol != "responses" {
		if effort != "" {
			payload["reasoning_effort"] = effort
		}
		return nil
	}
	if effort != "" {
		reasoning["effort"] = effort
	}
	if modelInfo.Reasoning && effort != "none" && modelInfo.ReasoningSummary != "" {
		if _, supplied := reasoning["summary"]; !supplied {
			reasoning["summary"] = modelInfo.ReasoningSummary
		}
	}
	if len(reasoning) > 0 {
		payload["reasoning"] = reasoning
	}
	return nil
}
