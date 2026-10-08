package spacedrepetition

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
)

type intervalConfiguration struct {
	Mode      string  `json:"mode"`
	Tokens    int     `json:"tokens"`
	Fraction  float64 `json:"fraction"`
	MaxTokens int     `json:"max_tokens"`
}

type configuration struct {
	Enabled                bool                  `json:"enabled"`
	Interval               intervalConfiguration `json:"interval"`
	Pattern                []string              `json:"pattern"`
	WorkerModel            string                `json:"worker_model"`
	WorkerEffort           string                `json:"worker_effort"`
	WorkerOutputTokens     int                   `json:"worker_output_tokens"`
	JobTimeoutMilliseconds int                   `json:"job_timeout_ms"`
	MaxQueries             int                   `json:"max_queries"`
	MemoryResultLimit      int                   `json:"memory_result_limit"`
	MemoryBytes            int                   `json:"memory_bytes"`
	SourceBytes            int                   `json:"source_bytes"`
	WorkerCount            int                   `json:"worker_count"`
}

func defaultConfiguration() configuration {
	return configuration{Interval: intervalConfiguration{Mode: "model_fraction", Tokens: 32768, Fraction: 0.125, MaxTokens: 32768}, Pattern: []string{"low", "low", "low", "medium"}, WorkerOutputTokens: 1024, JobTimeoutMilliseconds: 120000, MaxQueries: 4, MemoryResultLimit: 5, MemoryBytes: 12000, SourceBytes: 8000, WorkerCount: 2}
}

func mergeConfiguration(global, override json.RawMessage) (configuration, error) {
	encoded, _ := json.Marshal(defaultConfiguration())
	values := map[string]json.RawMessage{}
	if operationError := json.Unmarshal(encoded, &values); operationError != nil {
		return configuration{}, operationError
	}
	for _, data := range []json.RawMessage{global, override} {
		if len(data) == 0 {
			continue
		}
		var fields map[string]json.RawMessage
		if operationError := json.Unmarshal(data, &fields); operationError != nil || fields == nil {
			return configuration{}, fmt.Errorf("spaced repetition settings must be a JSON object")
		}
		for key, value := range fields {
			if _, found := values[key]; !found {
				return configuration{}, fmt.Errorf("unknown spaced repetition setting %q", key)
			}
			if string(value) == "null" {
				continue
			}
			if key == "interval" {
				var current, updates map[string]json.RawMessage
				if json.Unmarshal(values[key], &current) != nil || json.Unmarshal(value, &updates) != nil || updates == nil {
					return configuration{}, fmt.Errorf("interval must be an object")
				}
				for name, field := range updates {
					if _, found := current[name]; !found {
						return configuration{}, fmt.Errorf("unknown interval setting %q", name)
					}
					if string(field) != "null" {
						current[name] = field
					}
				}
				value, _ = json.Marshal(current)
			}
			values[key] = value
		}
	}
	encoded, operationError := json.Marshal(values)
	if operationError != nil {
		return configuration{}, operationError
	}
	var result configuration
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if operationError := decoder.Decode(&result); operationError != nil {
		return result, operationError
	}
	return result, validateConfiguration(result)
}

func validateConfiguration(value configuration) error {
	if value.Interval.Mode != "tokens" && value.Interval.Mode != "model_fraction" {
		return fmt.Errorf("interval.mode must be tokens or model_fraction")
	}
	if value.Interval.Tokens < 256 || value.Interval.Tokens > 2097152 || value.Interval.MaxTokens < 256 || value.Interval.MaxTokens > 2097152 {
		return fmt.Errorf("interval token values are outside 256..2097152")
	}
	if math.IsNaN(value.Interval.Fraction) || math.IsInf(value.Interval.Fraction, 0) || value.Interval.Fraction < 0.01 || value.Interval.Fraction > 0.5 {
		return fmt.Errorf("interval.fraction is outside 0.01..0.5")
	}
	if len(value.Pattern) < 1 || len(value.Pattern) > 32 {
		return fmt.Errorf("pattern requires 1 to 32 levels")
	}
	for _, level := range value.Pattern {
		if level != "low" && level != "medium" {
			return fmt.Errorf("pattern level %q is not low or medium", level)
		}
	}
	if value.WorkerOutputTokens < 256 || value.WorkerOutputTokens > 4096 || value.JobTimeoutMilliseconds < 1000 || value.JobTimeoutMilliseconds > 600000 {
		return fmt.Errorf("worker output or timeout is outside supported bounds")
	}
	if value.MaxQueries < 1 || value.MaxQueries > 8 || value.MemoryResultLimit < 1 || value.MemoryResultLimit > 20 || value.WorkerCount < 1 || value.WorkerCount > 4 {
		return fmt.Errorf("worker search or concurrency limits are outside supported bounds")
	}
	if value.MemoryBytes < 1024 || value.MemoryBytes > 16384 || value.SourceBytes < 1024 || value.SourceBytes > 16000 {
		return fmt.Errorf("worker source byte budgets are outside supported bounds")
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

func configurationEqual(first, second configuration) bool { return reflect.DeepEqual(first, second) }

func patchConfiguration(previous string, patch json.RawMessage) (json.RawMessage, error) {
	fields := map[string]json.RawMessage{}
	if previous != "" {
		if operationError := json.Unmarshal([]byte(previous), &fields); operationError != nil {
			return nil, operationError
		}
	}
	var updates map[string]json.RawMessage
	if json.Unmarshal(patch, &updates) != nil || updates == nil {
		return nil, fmt.Errorf("plugin configuration update must be a JSON object")
	}
	for key, value := range updates {
		if string(value) == "null" {
			delete(fields, key)
			continue
		}
		if key == "interval" {
			current := map[string]json.RawMessage{}
			var nested map[string]json.RawMessage
			if len(fields[key]) > 0 {
				if json.Unmarshal(fields[key], &current) != nil {
					return nil, fmt.Errorf("stored interval is not an object")
				}
			}
			if json.Unmarshal(value, &nested) != nil || nested == nil {
				return nil, fmt.Errorf("interval must be an object")
			}
			for name, field := range nested {
				if string(field) == "null" {
					delete(current, name)
				} else {
					current[name] = field
				}
			}
			value, _ = json.Marshal(current)
		}
		fields[key] = value
	}
	return json.Marshal(fields)
}
