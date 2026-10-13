package sidekick

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
	configuration, override, operationError := plugin.loadConfiguration(operationContext, workspaceID)
	if operationError != nil {
		return harness.PluginState{}, operationError
	}
	encoded, _ := json.Marshal(configuration)
	state := harness.PluginState{Name: pluginName, Version: plugin.Version(), WorkspaceID: workspaceID, Available: plugin.unavailable == nil, Enabled: configuration.Enabled, RequestedEnabled: configuration.Enabled, Configuration: encoded, Override: override, Schema: configurationSchema()}
	if plugin.unavailable != nil {
		state.Error = plugin.unavailable.Error()
	}
	plugin.mutex.Lock()
	defer plugin.mutex.Unlock()
	if scope := plugin.scopes[workspaceID]; scope != nil && scope.initialized {
		state.Enabled = scope.configuration.Enabled
		state.Pending = !configurationEqual(scope.configuration, configuration) || (!configuration.Enabled && scope.running > 0)
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
	configuration, operationError := mergeConfiguration(global, override)
	if operationError != nil {
		return harness.PluginState{}, operationError
	}
	if configuration.Enabled && plugin.unavailable != nil {
		return harness.PluginState{}, plugin.unavailable
	}
	if configuration.WorkerModel != "" {
		if _, operationError := plugin.services.Models.Model(operationContext, workspaceID, configuration.WorkerModel, configuration.WorkerEffort); operationError != nil {
			return harness.PluginState{}, operationError
		}
	}
	if operationError := plugin.services.Settings.Save(operationContext, workspaceID, settingsKey, string(data)); operationError != nil {
		return harness.PluginState{}, operationError
	}
	plugin.mutex.Lock()
	identifiers := []string{}
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
		if !configurationEqual(scope.configuration, desired) {
			for _, job := range plugin.jobs {
				if job.latest.session.InstanceID == identifier {
					job.cancel()
				}
			}
		}
		if scope.requests == 0 {
			scope.configuration = desired
			scope.initialized = true
			scope.pending = nil
		} else {
			scope.pending = &desired
		}
		scope.lastError = ""
		plugin.signalChangedLocked()
		plugin.mutex.Unlock()
	}
	return plugin.readState(operationContext, workspaceID)
}

func (plugin *Plugin) Metrics(operationContext context.Context, workspaceID string) (harness.PluginMetrics, error) {
	result := harness.PluginMetrics{Name: pluginName, WorkspaceID: workspaceID, Counters: map[string]int64{}}
	if plugin.database != nil {
		counts, operationError := plugin.database.Counters(operationContext, workspaceID)
		if operationError != nil {
			return result, operationError
		}
		result.Counters = counts
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
	plugin.mutex.Lock()
	pending := int64(0)
	for _, job := range plugin.jobs {
		if job.latest.session.InstanceID == workspaceID {
			pending++
		}
	}
	plugin.mutex.Unlock()
	result.Counters["jobs/active"] = pending
	return result, nil
}
