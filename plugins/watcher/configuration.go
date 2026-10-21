package watcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func applyDefaults(value *definition) {
	if value.Prompt == "" {
		value.Prompt = defaultPrompt
	}
	if value.IntervalTokens == 0 {
		value.IntervalTokens = 32768
	}
	if value.ContextTokens == 0 {
		value.ContextTokens = 32768
	}
	if value.TimeoutMS == 0 {
		value.TimeoutMS = 120000
	}
	if value.MaxRounds == 0 {
		value.MaxRounds = 12
	}
	if value.MaxOutputTokens == 0 {
		value.MaxOutputTokens = 2048
	}
	if value.MaxContinuations == 0 {
		value.MaxContinuations = 3
	}
}
func mergeConfiguration(global, workspace json.RawMessage) (configuration, error) {
	values := map[string]json.RawMessage{}
	for _, data := range []json.RawMessage{global, workspace} {
		if len(data) == 0 {
			continue
		}
		var fields map[string]json.RawMessage
		if operationError := json.Unmarshal(data, &fields); operationError != nil {
			return configuration{}, operationError
		}
		for key, value := range fields {
			if string(value) != "null" {
				values[key] = value
			}
		}
	}
	data, operationError := json.Marshal(values)
	if operationError != nil {
		return configuration{}, operationError
	}
	var result configuration
	if operationError = json.Unmarshal(data, &result); operationError != nil {
		return result, operationError
	}
	if result.Watchers == nil {
		result.Watchers = []definition{}
	}
	for index := range result.Watchers {
		applyDefaults(&result.Watchers[index])
	}
	return result, nil
}
func (plugin *Plugin) loadConfiguration(operationContext context.Context, workspaceID string) (configuration, json.RawMessage, error) {
	return harness.LoadPluginConfiguration(operationContext, plugin.services.Settings, workspaceID, settingsKey, mergeConfiguration)
}
func definitionSignature(value definition) string {
	data, _ := json.Marshal(value)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
func (plugin *Plugin) validateConfiguration(operationContext context.Context, workspaceID string, value configuration) error {
	var available []harness.WorkerTool
	if workspaceID != "" {
		var operationError error
		available, operationError = plugin.services.Sessions.Tools(operationContext, workspaceID)
		if operationError != nil {
			return operationError
		}
	}
	tools := map[string]harness.WorkerTool{}
	for _, tool := range available {
		tools[tool.Name] = tool
	}
	identifiers := map[string]bool{}
	for _, watcher := range value.Watchers {
		if len(watcher.ID) < 1 || len(watcher.ID) > 100 || identifiers[watcher.ID] || strings.ContainsAny(watcher.ID, "\r\n\x00") {
			return fmt.Errorf("watcher ID is empty, repeated or invalid")
		}
		identifiers[watcher.ID] = true
		if strings.TrimSpace(watcher.Name) == "" || len(watcher.Name) > 200 {
			return fmt.Errorf("watcher name must contain 1-200 bytes")
		}
		if !utf8.ValidString(watcher.Prompt) || len(watcher.Prompt) > 65536 {
			return fmt.Errorf("watcher prompt must be UTF-8 and at most 65536 bytes")
		}
		if operationError := validateTemplate(watcher.Prompt); operationError != nil {
			return operationError
		}
		if len(watcher.Triggers) == 0 {
			return fmt.Errorf("watcher requires at least one trigger")
		}
		seen := map[string]bool{}
		for _, trigger := range watcher.Triggers {
			if !slices.Contains([]string{"context_growth", "permission_request", "end_turn"}, trigger) || seen[trigger] {
				return fmt.Errorf("invalid or repeated watcher trigger %q", trigger)
			}
			seen[trigger] = true
		}
		if watcher.IntervalTokens < 256 || watcher.IntervalTokens > 1048576 || watcher.ContextTokens < 256 || watcher.ContextTokens > 1048576 {
			return fmt.Errorf("watcher token settings must be between 256 and 1048576")
		}
		if watcher.TimeoutMS < 1000 || watcher.TimeoutMS > 900000 || watcher.MaxRounds < 1 || watcher.MaxRounds > 100 || watcher.MaxOutputTokens < 128 || watcher.MaxOutputTokens > 65536 || watcher.MaxContinuations < 1 || watcher.MaxContinuations > 100 {
			return fmt.Errorf("watcher execution limits are invalid")
		}
		selected := map[string]bool{}
		for _, grant := range watcher.Tools {
			if selected[grant.Name] || slices.Contains([]string{"continue", "interfere", "search_tool", "finish"}, grant.Name) {
				return fmt.Errorf("invalid or repeated watcher tool %q", grant.Name)
			}
			selected[grant.Name] = true
			if workspaceID == "" || !value.Enabled || !watcher.Enabled {
				continue
			}
			tool, found := tools[grant.Name]
			if !found {
				return fmt.Errorf("watcher tool %q is not registered", grant.Name)
			}
			if len(tool.Operations) > 0 && len(grant.Operations) == 0 {
				return fmt.Errorf("tool %q requires operation selection", grant.Name)
			}
			operations := map[string]bool{}
			for _, operation := range grant.Operations {
				if !slices.Contains(tool.Operations, operation) || operations[operation] {
					return fmt.Errorf("invalid operation %q for tool %q", operation, grant.Name)
				}
				operations[operation] = true
			}
		}
		if watcher.Enabled && value.Enabled {
			if watcher.Model == "" {
				return fmt.Errorf("enabled watcher %q requires a model", watcher.Name)
			}
			model, operationError := plugin.services.Models.Model(operationContext, workspaceID, watcher.Model, watcher.ReasoningEffort)
			if operationError != nil {
				return operationError
			}
			if !model.Tools && !model.ToolSupportUnknown {
				return fmt.Errorf("watcher model does not support tools")
			}
		}
	}
	return nil
}
