package loop

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (agentLoop *Loop) resolveModel(instanceID, modelID string) (atom.ModelInfo, harness.Provider, error) {
	if agentLoop.configuration.Instances == nil {
		return agentLoop.configuration.Gateway.Resolve(modelID)
	}
	instance, found := agentLoop.configuration.Instances.Get(instanceID)
	if !found {
		return atom.ModelInfo{}, nil, fmt.Errorf("instance is stopped")
	}
	return agentLoop.configuration.Gateway.ResolveAllowed(modelID, instance.Spec().Models)
}

func (agentLoop *Loop) addAgentModelChoices(session atom.Session, specification atom.ToolSpec) (atom.ToolSpec, error) {
	var schema map[string]any
	if operationError := json.Unmarshal(specification.InputSchema.JSON, &schema); operationError != nil {
		return specification, operationError
	}
	properties, found := schema["properties"].(map[string]any)
	if !found {
		return specification, nil
	}
	var allowed []string
	var defaultModel string
	if agentLoop.configuration.Instances != nil {
		if instance, found := agentLoop.configuration.Instances.Get(session.InstanceID); found {
			allowed = instance.Spec().Models
			defaultModel = instance.Spec().DefaultModel
		}
	}
	var identifiers []string
	var descriptions []string
	for _, provider := range agentLoop.configuration.Gateway.Providers() {
		for _, model := range provider.Models() {
			identifier := provider.Name() + "/" + model.ID
			if _, _, operationError := agentLoop.configuration.Gateway.ResolveAllowed(identifier, allowed); operationError != nil {
				continue
			}
			identifiers = append(identifiers, identifier)
			description := fmt.Sprintf("%s: level %d", identifier, model.Level)
			if model.Prices != nil {
				description += fmt.Sprintf(", %s input %.6g/output %.6g per million tokens", model.Prices.Currency, model.Prices.Input, model.Prices.Output)
			}
			if len(model.ReasoningEfforts) > 0 {
				description += ", reasoning efforts " + strings.Join(model.ReasoningEfforts, "/")
			}
			if model.Billing == "subscription" {
				description += ", subscription billing"
			}
			descriptions = append(descriptions, description)
		}
	}
	modelProperty, found := properties["model"].(map[string]any)
	if !found {
		modelProperty = map[string]any{"type": "string"}
	}
	// Add live choices to the tool's contract without discarding its instructions
	// or other schema annotations when the session's model catalog changes.
	description, _ := modelProperty["description"].(string)
	if len(descriptions) > 0 {
		description = strings.TrimSpace(description + " Available models: " + strings.Join(descriptions, "; ") + ".")
	}
	modelProperty["description"] = description
	modelProperty["enum"] = identifiers
	if defaultModel != "" {
		if model, provider, operationError := agentLoop.configuration.Gateway.ResolveAllowed(defaultModel, allowed); operationError == nil {
			modelProperty["default"] = provider.Name() + "/" + model.ID
		}
	}
	properties["model"] = modelProperty
	data, operationError := json.Marshal(schema)
	if operationError != nil {
		return specification, operationError
	}
	specification.InputSchema.JSON = data
	return specification, nil
}

func validateModelMedia(modelInfo atom.ModelInfo, messages []atom.Message) error {
	if len(modelInfo.Input) == 0 {
		modelInfo.Input = []atom.MediaType{atom.Text}
	}
	allowed := map[atom.MediaType]bool{}
	for _, media := range modelInfo.Input {
		allowed[media] = true
	}
	for _, message := range messages {
		for _, item := range message.Content {
			if item.Type == "" {
				continue
			}
			if !allowed[item.Type] {
				return fmt.Errorf("loop: the model %s does not accept the media type %s", modelInfo.ID, item.Type)
			}
		}
	}
	return nil
}
