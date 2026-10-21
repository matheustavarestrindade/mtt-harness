package watcher

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (plugin *Plugin) Action(operationContext context.Context, workspaceID, action string, input json.RawMessage) (any, error) {
	if plugin.database == nil {
		return nil, plugin.unavailable
	}
	switch action {
	case "runs":
		var query struct {
			SessionID string `json:"session_id"`
			Cursor    string `json:"cursor"`
			Limit     int    `json:"limit"`
		}
		if len(input) > 0 {
			if operationError := json.Unmarshal(input, &query); operationError != nil {
				return nil, operationError
			}
		}
		if query.Limit == 0 {
			query.Limit = 50
		}
		return plugin.database.Runs(operationContext, workspaceID, query.SessionID, query.Cursor, query.Limit)
	case "cancel":
		var query struct {
			RunID string `json:"run_id"`
		}
		if operationError := json.Unmarshal(input, &query); operationError != nil {
			return nil, operationError
		}
		plugin.mutex.Lock()
		defer plugin.mutex.Unlock()
		for _, group := range plugin.active {
			if group.workspaceID == workspaceID && group.runs[query.RunID] {
				group.cancel()
				return map[string]bool{"cancelled": true}, nil
			}
		}
		return map[string]bool{"cancelled": false}, nil
	case "preview":
		var query struct {
			SessionID atom.SessionID `json:"session_id"`
			Watcher   definition     `json:"watcher"`
			Trigger   string         `json:"trigger"`
		}
		if operationError := json.Unmarshal(input, &query); operationError != nil {
			return nil, operationError
		}
		applyDefaults(&query.Watcher)
		query.Watcher.Enabled = true
		if operationError := plugin.validateConfiguration(operationContext, workspaceID, configuration{Enabled: true, Watchers: []definition{query.Watcher}}); operationError != nil {
			return nil, operationError
		}
		source, operationError := plugin.services.Sessions.Source(operationContext, query.SessionID, min(4<<20, max(4096, templateMaximumTokens(query.Watcher)*6)), query.Watcher.IncludeReasoning)
		if operationError != nil {
			return nil, operationError
		}
		if source.Workspace.ID != workspaceID {
			return nil, fmt.Errorf("preview session is outside the workspace")
		}
		prompt, budget, truncated, operationError := plugin.renderInput(operationContext, query.Watcher, source, observation{Session: source.Session, Trigger: query.Trigger})
		if operationError != nil {
			return nil, operationError
		}
		return map[string]any{"prompt": boundedText(prompt, 65536), "truncated": truncated || len(prompt) > 65536, "input_tokens": budget.InputTokens, "input_limit": budget.InputLimit, "estimated": budget.Estimated}, nil
	default:
		return nil, harness.ErrPluginActionNotFound
	}
}

func (plugin *Plugin) SessionActivity(operationContext context.Context, session atom.Session, query harness.PluginActivityQuery) (harness.PluginActivityPage, error) {
	result := harness.PluginActivityPage{Name: pluginName, SessionID: session.ID, HistoryRevision: int64(session.HistoryRevision), Entries: []harness.PluginActivityEntry{}}
	if plugin.database == nil {
		return result, nil
	}
	page, state, operationError := plugin.database.Activity(operationContext, session.InstanceID, string(session.ID), query.Cursor, query.Limit)
	if operationError != nil {
		return result, operationError
	}
	result.Revision = state.Revision
	result.NextCursor = page.NextCursor
	for _, run := range page.Runs {
		entry := harness.PluginActivityEntry{ID: run.ID, SessionID: session.ID, AnchorID: run.AnchorID, AnchorSequence: run.AnchorSequence, Title: run.Name, CreatedAt: run.CreatedAt, Outcome: run.Status, Decision: run.Decision, Trigger: run.Trigger, DefinitionID: run.DefinitionID, WorkerSessionID: run.WorkerSessionID, DurationMS: run.DurationMS, Error: run.Error, Estimated: run.Estimated, InputPreview: run.InputPreview, InputTruncated: run.InputTruncated, Notes: []harness.PluginActivityNote{}}
		entry.ContinueChat = run.ContinueChat
		text := run.Message
		if text == "" {
			text = run.Reason
		}
		if text != "" {
			entry.Notes = []harness.PluginActivityNote{{Text: text}}
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}
