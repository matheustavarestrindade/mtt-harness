package contextplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (plugin *Plugin) BeginRequest(operationContext context.Context, session atom.Session) (context.Context, func(), error) {
	if operationError := operationContext.Err(); operationError != nil {
		return operationContext, nil, operationError
	}
	// A blocking child agent belongs to its parent's accepted tool group. It
	// must inherit that lease rather than wait for the parent to finish itself.
	if parent, found := operationContext.Value(requestContextKey{plugin}).(requestState); found && session.Parent != "" && parent.sessionID == session.Parent && parent.workspaceID == session.InstanceID {
		select {
		case <-parent.done:
		default:
			plugin.mutex.Lock()
			if plugin.closed {
				plugin.mutex.Unlock()
				return operationContext, nil, fmt.Errorf("context plugin is closed")
			}
			scope := plugin.scopeLocked(session.InstanceID)
			scope.requests++
			plugin.mutex.Unlock()
			return plugin.makeRequestLease(operationContext, session, parent.configuration, scope)
		}
	}
	for {
		// Serialize the settings read through admission with API updates. A
		// delayed reader must not restore a previous configuration.
		plugin.configurationMutex.Lock()
		configuration, _, operationError := plugin.loadConfiguration(operationContext, session.InstanceID)
		if operationError != nil {
			plugin.configurationMutex.Unlock()
			return operationContext, nil, operationError
		}
		if configuration.Enabled && plugin.unavailable != nil {
			plugin.configurationMutex.Unlock()
			return operationContext, nil, plugin.unavailable
		}
		plugin.mutex.Lock()
		if plugin.closed {
			plugin.mutex.Unlock()
			plugin.configurationMutex.Unlock()
			return operationContext, nil, fmt.Errorf("context plugin is closed")
		}
		scope := plugin.scopeLocked(session.InstanceID)
		if !scope.initialized || !configurationEqual(scope.configuration, configuration) {
			if scope.requests > 0 {
				desired := configuration
				scope.pending = &desired
				changed := scope.changed
				plugin.mutex.Unlock()
				plugin.configurationMutex.Unlock()
				select {
				case <-changed:
					continue
				case <-operationContext.Done():
					return operationContext, nil, operationContext.Err()
				}
			}
			plugin.applyConfigurationLocked(scope, configuration)
		}
		scope.requests++
		plugin.mutex.Unlock()
		plugin.configurationMutex.Unlock()
		return plugin.makeRequestLease(operationContext, session, configuration, scope)
	}
}

func (plugin *Plugin) makeRequestLease(operationContext context.Context, session atom.Session, configuration configuration, scope *workspaceRuntime) (context.Context, func(), error) {
	var once sync.Once
	done := make(chan struct{})
	release := func() {
		once.Do(func() {
			plugin.mutex.Lock()
			close(done)
			scope.requests--
			if scope.requests == 0 && scope.pending != nil {
				plugin.applyConfigurationLocked(scope, *scope.pending)
			}
			plugin.mutex.Unlock()
			plugin.signalWork()
		})
	}
	return context.WithValue(operationContext, requestContextKey{plugin}, requestState{workspaceID: session.InstanceID, sessionID: session.ID, configuration: configuration, done: done}), release, nil
}

func (plugin *Plugin) requestConfiguration(operationContext context.Context, workspaceID string) (configuration, error) {
	if frozen, found := operationContext.Value(requestContextKey{plugin}).(requestState); found && frozen.workspaceID == workspaceID {
		return frozen.configuration, nil
	}
	configuration, _, operationError := plugin.loadConfiguration(operationContext, workspaceID)
	return configuration, operationError
}

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
	state := harness.PluginState{Name: pluginName, Version: pluginVersion, WorkspaceID: workspaceID, Available: plugin.unavailable == nil && plugin.services.Embeddings != nil, RequestedEnabled: configuration.Enabled, Enabled: configuration.Enabled, Configuration: encoded, Override: override, Schema: configurationSchema()}
	if plugin.unavailable != nil {
		state.Error = plugin.unavailable.Error()
	}
	if plugin.services.Embeddings == nil && state.Error == "" {
		state.Error = "semantic embeddings are unavailable in this build"
	}
	plugin.mutex.Lock()
	if plugin.lastError != "" && state.Error == "" {
		state.Error = plugin.lastError
	}
	if scope := plugin.scopes[workspaceID]; scope != nil && scope.initialized {
		state.Enabled = scope.configuration.Enabled
		state.Pending = !configurationEqual(scope.configuration, configuration) || (!configuration.Enabled && len(scope.jobs) > 0)
	}
	plugin.mutex.Unlock()
	return state, nil
}

func (plugin *Plugin) UpdateConfiguration(operationContext context.Context, workspaceID string, patch json.RawMessage) (harness.PluginState, error) {
	plugin.configurationMutex.Lock()
	defer plugin.configurationMutex.Unlock()
	var updates map[string]json.RawMessage
	if operationError := json.Unmarshal(patch, &updates); operationError != nil || updates == nil {
		return harness.PluginState{}, fmt.Errorf("plugin configuration update must be a JSON object")
	}
	old, operationError := plugin.services.Settings.Get(operationContext, workspaceID, settingsKey)
	if operationError != nil {
		return harness.PluginState{}, operationError
	}
	fields := map[string]json.RawMessage{}
	if old != "" {
		if operationError := json.Unmarshal([]byte(old), &fields); operationError != nil {
			return harness.PluginState{}, operationError
		}
	}
	for key, value := range updates {
		if string(value) == "null" {
			delete(fields, key)
		} else {
			fields[key] = value
		}
	}
	data, operationError := json.Marshal(fields)
	if operationError != nil {
		return harness.PluginState{}, operationError
	}
	global := data
	var override json.RawMessage
	if workspaceID != "" {
		globalValue, operationError := plugin.services.Settings.Get(operationContext, "", settingsKey)
		if operationError != nil {
			return harness.PluginState{}, operationError
		}
		global = json.RawMessage(globalValue)
		override = data
	}
	configuration, operationError := mergeConfiguration(global, override)
	if operationError != nil {
		return harness.PluginState{}, operationError
	}
	if operationError := plugin.validateServices(operationContext, workspaceID, configuration); operationError != nil {
		return harness.PluginState{}, operationError
	}
	if operationError := plugin.services.Settings.Save(operationContext, workspaceID, settingsKey, string(data)); operationError != nil {
		return harness.PluginState{}, operationError
	}
	plugin.mutex.Lock()
	var affected []string
	for identifier := range plugin.scopes {
		if workspaceID == "" || identifier == workspaceID {
			affected = append(affected, identifier)
		}
	}
	plugin.mutex.Unlock()
	for _, identifier := range affected {
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
	if workspaceID != "" && plugin.database != nil {
		if operationError := plugin.database.Transact(operationContext, workspaceID, func(database transaction) error {
			return database.Put("workspace", "state", map[string]any{"id": workspaceID})
		}); operationError != nil {
			return harness.PluginState{}, operationError
		}
	}
	plugin.signalWork()
	return plugin.readState(operationContext, workspaceID)
}

func configurationSchema() atom.Schema {
	return atom.Schema{JSON: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{
"enabled":{"type":["boolean","null"],"description":"Enable workspace memory. Default false. A workspace value overrides the global value; null removes that override. Applied after active tool groups."},
"worker_model":{"type":["string","null"],"description":"Provider-qualified model ID for memory extraction and remember. Required when enabled; no model is selected by default."},
"worker_effort":{"type":["string","null"],"description":"Reasoning effort accepted by worker model metadata. Empty uses its model default."},
"historian_model":{"type":["string","null"],"description":"Provider-qualified historian model. Empty uses worker_model."},
"historian_effort":{"type":["string","null"],"description":"Historian reasoning effort. Empty uses its model default; when historian_model is empty, worker effort is inherited."},
"review_percent":{"type":["integer","null"],"description":"Input-budget percentage for the first context notice. Default 50; target < review < urgent < hard."},
"urgent_percent":{"type":["integer","null"],"description":"Input-budget percentage for an urgent wrap-up notice. Default 65."},
"hard_percent":{"type":["integer","null"],"description":"Input-budget percentage at which the next request waits for forced compaction. Default 80; maximum 90."},
"target_percent":{"type":["integer","null"],"description":"Post-compaction input-budget target percentage. Default 45; 10..49, below review_percent."},
"snapshot_tokens":{"type":["integer","null"],"description":"Estimated token limit for the memory snapshot. Default 1000; 128..8192. The final model input budget still applies."},
"worker_count":{"type":["integer","null"],"description":"Maximum simultaneous memory jobs per workspace. Default 2; 1..8."},
"retry_limit":{"type":["integer","null"],"description":"Retries after a failed memory job attempt. Default 2; 0..5. Each model attempt is billed separately."},
"job_timeout_ms":{"type":["integer","null"],"description":"Memory job attempt timeout in milliseconds. Default 120000; 1000..600000. Cancellation stops subsequent model and embedding work."},
"worker_output_tokens":{"type":["integer","null"],"description":"Maximum worker response tokens. Default 4096; 512..16384, subject to its model context."},
"source_batch_bytes":{"type":["integer","null"],"description":"Maximum UTF-8 source bytes per worker chunk before model-budget fitting. Default 16000; 1024..64000. Every chunk is retained."},
"historian_idle_hours":{"type":["integer","null"],"description":"Hours without main-agent retrieval before consolidation is eligible. Default 72; 1..8760. Worker reads do not reset this clock."},
"historian_interval_minutes":{"type":["integer","null"],"description":"Minimum minutes between historian scans per workspace. Default 60; 1..10080."},
"historian_minimum_group":{"type":["integer","null"],"description":"Minimum related cold memory records per idea. Default 4; 2..16."},
"cost_limit":{"type":["number","null"],"description":"Workspace context-agent lifetime cost ceiling in cost_currency. Default zero means no automatic cost limit. Unknown prices cannot establish a finite budget."},
"cost_currency":{"type":["string","null"],"description":"Three-letter currency for cost_limit. Default USD; rates must have the same currency."}
}}`)}
}
