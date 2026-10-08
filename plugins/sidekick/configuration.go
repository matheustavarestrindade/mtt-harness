package sidekick

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

type configuration struct {
	Enabled                bool   `json:"enabled"`
	WorkerModel            string `json:"worker_model"`
	WorkerEffort           string `json:"worker_effort"`
	MemoryEnabled          bool   `json:"memory_enabled"`
	FilesEnabled           bool   `json:"files_enabled"`
	DebounceMilliseconds   int    `json:"debounce_ms"`
	CooldownMilliseconds   int    `json:"cooldown_ms"`
	JobTimeoutMilliseconds int    `json:"job_timeout_ms"`
	WorkerCount            int    `json:"worker_count"`
	WorkerOutputTokens     int    `json:"worker_output_tokens"`
	MemoryLimit            int    `json:"memory_limit"`
	MemoryBytes            int    `json:"memory_bytes"`
	FileLimit              int    `json:"file_limit"`
	FileBytes              int    `json:"file_bytes"`
	SourceBytes            int    `json:"source_bytes"`
	HintBytes              int    `json:"hint_bytes"`
}

func defaultConfiguration() configuration {
	return configuration{MemoryEnabled: true, FilesEnabled: true, DebounceMilliseconds: 750, CooldownMilliseconds: 15000, JobTimeoutMilliseconds: 45000, WorkerCount: 1, WorkerOutputTokens: 768, MemoryLimit: 5, MemoryBytes: 6000, FileLimit: 5, FileBytes: 8000, SourceBytes: 4000, HintBytes: 1600}
}

func mergeConfiguration(global, override json.RawMessage) (configuration, error) {
	defaults, _ := json.Marshal(defaultConfiguration())
	fields := map[string]json.RawMessage{}
	_ = json.Unmarshal(defaults, &fields)
	for _, data := range []json.RawMessage{global, override} {
		if len(data) == 0 {
			continue
		}
		var updates map[string]json.RawMessage
		if json.Unmarshal(data, &updates) != nil || updates == nil {
			return configuration{}, fmt.Errorf("sidekick settings must be a JSON object")
		}
		for name, value := range updates {
			if _, found := fields[name]; !found {
				return configuration{}, fmt.Errorf("unknown sidekick setting %q", name)
			}
			if string(value) != "null" {
				fields[name] = value
			}
		}
	}
	data, _ := json.Marshal(fields)
	var value configuration
	if operationError := json.Unmarshal(data, &value); operationError != nil {
		return value, operationError
	}
	return value, validateConfiguration(value)
}

func validateConfiguration(value configuration) error {
	if value.Enabled && value.WorkerModel == "" {
		return fmt.Errorf("sidekick worker_model is empty")
	}
	if value.Enabled && !value.MemoryEnabled && !value.FilesEnabled {
		return fmt.Errorf("sidekick has no enabled retrieval source")
	}
	if value.DebounceMilliseconds < 0 || value.DebounceMilliseconds > 10000 || value.CooldownMilliseconds < 0 || value.CooldownMilliseconds > 300000 {
		return fmt.Errorf("sidekick debounce or cooldown is outside supported millisecond bounds")
	}
	if value.JobTimeoutMilliseconds < 1000 || value.JobTimeoutMilliseconds > 300000 || value.WorkerCount < 1 || value.WorkerCount > 4 {
		return fmt.Errorf("sidekick timeout or concurrency is outside supported bounds")
	}
	if value.WorkerOutputTokens < 128 || value.WorkerOutputTokens > 2048 || value.MemoryLimit < 1 || value.MemoryLimit > 12 || value.FileLimit < 1 || value.FileLimit > 12 {
		return fmt.Errorf("sidekick output or retrieval count is outside supported bounds")
	}
	if value.MemoryBytes < 256 || value.MemoryBytes > 16384 || value.FileBytes < 256 || value.FileBytes > 16384 || value.SourceBytes < 256 || value.SourceBytes > 8000 || value.HintBytes < 128 || value.HintBytes > 4096 {
		return fmt.Errorf("sidekick byte budgets are outside supported bounds")
	}
	return nil
}

func (plugin *Plugin) loadConfiguration(operationContext context.Context, workspaceID string) (configuration, json.RawMessage, error) {
	global, operationError := plugin.services.Settings.Get(operationContext, "", settingsKey)
	if operationError != nil {
		return configuration{}, nil, operationError
	}
	var override string
	if workspaceID != "" {
		override, operationError = plugin.services.Settings.Get(operationContext, workspaceID, settingsKey)
		if operationError != nil {
			return configuration{}, nil, operationError
		}
	}
	value, operationError := mergeConfiguration(json.RawMessage(global), json.RawMessage(override))
	data := json.RawMessage(override)
	if workspaceID == "" {
		data = json.RawMessage(global)
	}
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}
	return value, data, operationError
}

func patchConfiguration(previous string, patch json.RawMessage) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if previous != "" {
		if operationError := json.Unmarshal([]byte(previous), &fields); operationError != nil {
			return nil, operationError
		}
	}
	var updates map[string]json.RawMessage
	if json.Unmarshal(patch, &updates) != nil || updates == nil {
		return nil, fmt.Errorf("sidekick update must be a JSON object")
	}
	for name, value := range updates {
		if string(value) == "null" {
			delete(fields, name)
		} else {
			fields[name] = value
		}
	}
	return json.Marshal(fields)
}
func configurationEqual(first, second configuration) bool { return reflect.DeepEqual(first, second) }
