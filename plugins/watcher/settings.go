package watcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	data, _ := json.Marshal(configuration)
	var tools []harness.WorkerTool
	if workspaceID != "" {
		tools, operationError = plugin.services.Sessions.Tools(operationContext, workspaceID)
		if operationError != nil {
			return harness.PluginState{}, operationError
		}
	}
	state := harness.PluginState{Name: pluginName, Version: plugin.Version(), WorkspaceID: workspaceID, Available: plugin.unavailable == nil, Enabled: configuration.Enabled, RequestedEnabled: configuration.Enabled, Configuration: data, Override: override, Schema: configurationSchema(tools)}
	if plugin.unavailable != nil {
		state.Error = plugin.unavailable.Error()
	}
	plugin.mutex.Lock()
	defer plugin.mutex.Unlock()
	if state.Error == "" {
		state.Error = plugin.errors[workspaceID]
	}
	for _, group := range plugin.active {
		if group.workspaceID == workspaceID && !configuration.Enabled {
			state.Pending = true
		}
	}
	return state, nil
}
func (plugin *Plugin) UpdateConfiguration(operationContext context.Context, workspaceID string, patch json.RawMessage) (harness.PluginState, error) {
	plugin.configurationMutex.Lock()
	defer plugin.configurationMutex.Unlock()
	var fields map[string]json.RawMessage
	if operationError := json.Unmarshal(patch, &fields); operationError != nil {
		return harness.PluginState{}, operationError
	}
	for name := range fields {
		if name != "enabled" && name != "watchers" {
			return harness.PluginState{}, fmt.Errorf("unknown watcher configuration field %q", name)
		}
	}
	previous, operationError := plugin.services.Settings.Get(operationContext, workspaceID, settingsKey)
	if operationError != nil {
		return harness.PluginState{}, operationError
	}
	stored := map[string]json.RawMessage{}
	if previous != "" {
		if operationError = json.Unmarshal([]byte(previous), &stored); operationError != nil {
			return harness.PluginState{}, operationError
		}
	}
	for name, value := range fields {
		if string(value) == "null" {
			delete(stored, name)
		} else {
			stored[name] = value
		}
	}
	data, operationError := json.Marshal(stored)
	if operationError != nil {
		return harness.PluginState{}, operationError
	}
	var checked configuration
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if operationError = decoder.Decode(&checked); operationError != nil {
		return harness.PluginState{}, operationError
	}
	global := data
	var override json.RawMessage
	if workspaceID != "" {
		text, readError := plugin.services.Settings.Get(operationContext, "", settingsKey)
		if readError != nil {
			return harness.PluginState{}, readError
		}
		global = json.RawMessage(text)
		override = data
	}
	configuration, operationError := mergeConfiguration(global, override)
	if operationError != nil {
		return harness.PluginState{}, operationError
	}
	if configuration.Enabled && plugin.unavailable != nil {
		return harness.PluginState{}, plugin.unavailable
	}
	if operationError = plugin.validateConfiguration(operationContext, workspaceID, configuration); operationError != nil {
		return harness.PluginState{}, operationError
	}
	if operationError = plugin.services.Settings.Save(operationContext, workspaceID, settingsKey, string(data)); operationError != nil {
		return harness.PluginState{}, operationError
	}
	plugin.mutex.Lock()
	for key, value := range plugin.inbox {
		if workspaceID == "" || value.Session.InstanceID == workspaceID {
			delete(plugin.inbox, key)
		}
	}
	plugin.compactInboxLocked()
	for _, group := range plugin.active {
		if workspaceID == "" || group.workspaceID == workspaceID {
			group.cancel()
		}
	}
	delete(plugin.errors, workspaceID)
	plugin.mutex.Unlock()
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
		if strings.HasPrefix(agent.Agent, "watcher.") {
			result.Agents = append(result.Agents, agent)
		}
	}
	plugin.mutex.Lock()
	for _, group := range plugin.active {
		if group.workspaceID == workspaceID {
			result.Counters["active"]++
		}
	}
	for _, value := range plugin.inbox {
		if value.Session.InstanceID == workspaceID {
			result.Counters["waiting"]++
		}
	}
	plugin.mutex.Unlock()
	return result, nil
}
