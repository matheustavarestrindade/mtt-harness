package harness

import (
	"context"
	"encoding/json"
)

// LoadPluginConfiguration reads the authoritative harness/workspace layers and
// retains the selected raw override for the settings API. Plugins still own
// defaults, per-field inheritance, validation and their configuration types.
func LoadPluginConfiguration[Configuration any](operationContext context.Context, settings PluginSettings, workspaceID, settingsKey string, mergeConfiguration func(json.RawMessage, json.RawMessage) (Configuration, error)) (Configuration, json.RawMessage, error) {
	var emptyConfiguration Configuration
	globalConfiguration, operationError := settings.Get(operationContext, "", settingsKey)
	if operationError != nil {
		return emptyConfiguration, nil, operationError
	}
	var workspaceOverride string
	if workspaceID != "" {
		workspaceOverride, operationError = settings.Get(operationContext, workspaceID, settingsKey)
		if operationError != nil {
			return emptyConfiguration, nil, operationError
		}
	}
	configuration, operationError := mergeConfiguration(json.RawMessage(globalConfiguration), json.RawMessage(workspaceOverride))
	selectedOverride := json.RawMessage(workspaceOverride)
	if workspaceID == "" {
		selectedOverride = json.RawMessage(globalConfiguration)
	}
	if len(selectedOverride) == 0 {
		selectedOverride = json.RawMessage(`{}`)
	}
	return configuration, selectedOverride, operationError
}
