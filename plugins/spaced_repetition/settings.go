package spacedrepetition

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (plugin *Plugin) ReadState(operationContext context.Context, workspaceID string) (harness.PluginState, error) {
	plugin.configurationMutex.Lock()
	defer plugin.configurationMutex.Unlock()
	return plugin.readState(operationContext, workspaceID)
}

func (plugin *Plugin) readState(operationContext context.Context, workspaceID string) (harness.PluginState, error) {
	value, override, operationError := plugin.loadConfiguration(operationContext, workspaceID)
	if operationError != nil {
		return harness.PluginState{}, operationError
	}
	encoded, _ := json.Marshal(value)
	state := harness.PluginState{Name: pluginName, Version: plugin.Version(), WorkspaceID: workspaceID, Available: plugin.unavailable == nil, Enabled: value.Enabled, RequestedEnabled: value.Enabled, Configuration: encoded, Override: override, Schema: configurationSchema()}
	if plugin.unavailable != nil {
		state.Error = plugin.unavailable.Error()
	}
	plugin.mutex.Lock()
	defer plugin.mutex.Unlock()
	if scope := plugin.scopes[workspaceID]; scope != nil && scope.initialized {
		state.Enabled = scope.configuration.Enabled
		state.Pending = !configurationEqual(scope.configuration, value) || (!value.Enabled && len(scope.jobs) > 0)
		if state.Error == "" {
			state.Error = scope.lastError
		}
	}
	return state, nil
}

func (plugin *Plugin) UpdateConfiguration(operationContext context.Context, workspaceID string, patch json.RawMessage) (harness.PluginState, error) {
	plugin.configurationMutex.Lock()
	defer plugin.configurationMutex.Unlock()
	previous, operationError := plugin.services.Settings.Get(operationContext, workspaceID, settingsKey)
	if operationError != nil {
		return harness.PluginState{}, operationError
	}
	data, operationError := patchConfiguration(previous, patch)
	if operationError != nil {
		return harness.PluginState{}, operationError
	}
	global := data
	var override json.RawMessage
	if workspaceID != "" {
		stored, operationError := plugin.services.Settings.Get(operationContext, "", settingsKey)
		if operationError != nil {
			return harness.PluginState{}, operationError
		}
		global = json.RawMessage(stored)
		override = data
	}
	value, operationError := mergeConfiguration(global, override)
	if operationError != nil {
		return harness.PluginState{}, operationError
	}
	if value.Enabled && plugin.unavailable != nil {
		return harness.PluginState{}, plugin.unavailable
	}
	if strings.TrimSpace(value.WorkerModel) != "" {
		if _, operationError := plugin.services.Models.Model(operationContext, workspaceID, value.WorkerModel, value.WorkerEffort); operationError != nil {
			return harness.PluginState{}, operationError
		}
	}
	if operationError := plugin.services.Settings.Save(operationContext, workspaceID, settingsKey, string(data)); operationError != nil {
		return harness.PluginState{}, operationError
	}
	plugin.mutex.Lock()
	var identifiers []string
	for identifier := range plugin.scopes {
		if workspaceID == "" || workspaceID == identifier {
			identifiers = append(identifiers, identifier)
		}
	}
	plugin.mutex.Unlock()
	for _, identifier := range identifiers {
		desired, _, operationError := plugin.loadConfiguration(operationContext, identifier)
		if operationError != nil {
			return harness.PluginState{}, operationError
		}
		plugin.mutex.Lock()
		scope := plugin.scopeLocked(identifier)
		if scope.requests == 0 {
			plugin.applyConfigurationLocked(scope, desired)
		} else {
			scope.pending = &desired
		}
		plugin.mutex.Unlock()
	}
	return plugin.readState(operationContext, workspaceID)
}

func (plugin *Plugin) Metrics(operationContext context.Context, workspaceID string) (harness.PluginMetrics, error) {
	result := harness.PluginMetrics{Name: pluginName, WorkspaceID: workspaceID, Counters: map[string]int64{}}
	if plugin.database != nil {
		counters, operationError := plugin.database.Counters(operationContext, workspaceID)
		if operationError != nil {
			return result, operationError
		}
		result.Counters = counters
	}
	agents, operationError := plugin.services.Usage.Agents(operationContext, workspaceID)
	if operationError != nil {
		return result, operationError
	}
	for _, agent := range agents {
		if strings.HasPrefix(agent.Agent, pluginName+".") {
			result.Agents = append(result.Agents, agent)
		}
	}
	return result, nil
}
