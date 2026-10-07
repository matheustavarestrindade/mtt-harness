package contextplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// configuration is stored in the existing settings service. A workspace JSON
// object overrides individual global fields; null removes an override.
type configuration struct {
	Enabled                  bool    `json:"enabled"`
	WorkerModel              string  `json:"worker_model"`
	WorkerEffort             string  `json:"worker_effort"`
	HistorianModel           string  `json:"historian_model"`
	HistorianEffort          string  `json:"historian_effort"`
	ReviewPercent            int     `json:"review_percent"`
	UrgentPercent            int     `json:"urgent_percent"`
	HardPercent              int     `json:"hard_percent"`
	TargetPercent            int     `json:"target_percent"`
	SnapshotTokens           int     `json:"snapshot_tokens"`
	WorkerCount              int     `json:"worker_count"`
	RetryLimit               int     `json:"retry_limit"`
	JobTimeoutMilliseconds   int     `json:"job_timeout_ms"`
	WorkerOutputTokens       int     `json:"worker_output_tokens"`
	SourceBatchBytes         int     `json:"source_batch_bytes"`
	HistorianIdleHours       int     `json:"historian_idle_hours"`
	HistorianIntervalMinutes int     `json:"historian_interval_minutes"`
	HistorianMinimumGroup    int     `json:"historian_minimum_group"`
	CostLimit                float64 `json:"cost_limit"`
	CostCurrency             string  `json:"cost_currency"`
}

func defaultConfiguration() configuration {
	return configuration{ReviewPercent: 50, UrgentPercent: 65, HardPercent: 80, TargetPercent: 45, SnapshotTokens: 1000, WorkerCount: 2, RetryLimit: 2, JobTimeoutMilliseconds: 120000, WorkerOutputTokens: 4096, SourceBatchBytes: 16000, HistorianIdleHours: 72, HistorianIntervalMinutes: 60, HistorianMinimumGroup: 4, CostCurrency: "USD"}
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
		if operationError := json.Unmarshal(data, &fields); operationError != nil {
			return configuration{}, operationError
		}
		if fields == nil {
			return configuration{}, fmt.Errorf("plugin configuration must be a JSON object")
		}
		for key, value := range fields {
			if _, found := values[key]; !found {
				return configuration{}, fmt.Errorf("unknown context setting %q", key)
			}
			if string(value) != "null" {
				values[key] = value
			}
		}
	}
	encoded, operationError := json.Marshal(values)
	if operationError != nil {
		return configuration{}, operationError
	}
	var result configuration
	if operationError := json.Unmarshal(encoded, &result); operationError != nil {
		return result, operationError
	}
	return result, validateConfiguration(result)
}

func validateConfiguration(configuration configuration) error {
	if configuration.TargetPercent < 10 || configuration.TargetPercent >= 50 || configuration.TargetPercent >= configuration.ReviewPercent || configuration.ReviewPercent >= configuration.UrgentPercent || configuration.UrgentPercent >= configuration.HardPercent || configuration.HardPercent > 90 {
		return fmt.Errorf("context percentages require 10 <= target < 50 and target < review < urgent < hard <= 90")
	}
	if configuration.SnapshotTokens < 128 || configuration.SnapshotTokens > 8192 {
		return fmt.Errorf("snapshot_tokens is outside 128..8192")
	}
	if configuration.WorkerCount < 1 || configuration.WorkerCount > 8 || configuration.RetryLimit < 0 || configuration.RetryLimit > 5 {
		return fmt.Errorf("worker_count is outside 1..8 or retry_limit is outside 0..5")
	}
	if configuration.JobTimeoutMilliseconds < 1000 || configuration.JobTimeoutMilliseconds > 600000 {
		return fmt.Errorf("job_timeout_ms is outside 1000..600000")
	}
	if configuration.WorkerOutputTokens < 512 || configuration.WorkerOutputTokens > 16384 || configuration.SourceBatchBytes < 1024 || configuration.SourceBatchBytes > 64000 {
		return fmt.Errorf("worker_output_tokens is outside 512..16384 or source_batch_bytes is outside 1024..64000")
	}
	if configuration.HistorianIdleHours < 1 || configuration.HistorianIdleHours > 8760 || configuration.HistorianIntervalMinutes < 1 || configuration.HistorianIntervalMinutes > 10080 || configuration.HistorianMinimumGroup < 2 || configuration.HistorianMinimumGroup > 16 {
		return fmt.Errorf("historian limits are outside the supported ranges")
	}
	if math.IsNaN(configuration.CostLimit) || math.IsInf(configuration.CostLimit, 0) || configuration.CostLimit < 0 || len(configuration.CostCurrency) != 3 {
		return fmt.Errorf("cost_limit must be finite and non-negative; cost_currency requires three letters")
	}
	if configuration.Enabled && strings.TrimSpace(configuration.WorkerModel) == "" {
		return fmt.Errorf("enabled context memory requires worker_model")
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
	result, operationError := mergeConfiguration(json.RawMessage(global), json.RawMessage(override))
	data := json.RawMessage(override)
	if workspaceID == "" {
		data = json.RawMessage(global)
	}
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}
	return result, data, operationError
}

func configurationEqual(first, second configuration) bool {
	firstData, _ := json.Marshal(first)
	secondData, _ := json.Marshal(second)
	return bytes.Equal(firstData, secondData)
}

func (plugin *Plugin) validateServices(operationContext context.Context, workspaceID string, configuration configuration) error {
	if !configuration.Enabled {
		return nil
	}
	if plugin.unavailable != nil {
		return plugin.unavailable
	}
	if plugin.services.Embeddings == nil {
		return fmt.Errorf("semantic embeddings are unavailable in this build")
	}
	if _, operationError := plugin.services.Embeddings.Split(operationContext, "availability"); operationError != nil {
		return operationError
	}
	if _, operationError := plugin.services.Models.Model(operationContext, workspaceID, configuration.WorkerModel, configuration.WorkerEffort); operationError != nil {
		return operationError
	}
	model, effort := configuration.HistorianModel, configuration.HistorianEffort
	if model == "" {
		model, effort = configuration.WorkerModel, configuration.WorkerEffort
	}
	_, operationError := plugin.services.Models.Model(operationContext, workspaceID, model, effort)
	return operationError
}
