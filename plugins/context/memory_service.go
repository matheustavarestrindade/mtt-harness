package contextplugin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

// SearchWorkspaceMemory gives other plugins current, scoped memory references.
// It records attributed retrieval work without touching any conversation view.
func (plugin *Plugin) SearchWorkspaceMemory(operationContext context.Context, input harness.MemoryQuery) ([]harness.MemoryReference, error) {
	if input.WorkspaceID == "" || input.Agent == "" || input.RunID == "" {
		return nil, fmt.Errorf("memory retrieval requires workspace, agent, and run IDs")
	}
	if _, operationError := plugin.services.Workspaces.Workspace(operationContext, input.WorkspaceID); operationError != nil {
		return nil, operationError
	}
	if input.SourceSessionID != "" {
		session, operationError := plugin.services.Conversations.Get(operationContext, input.SourceSessionID)
		if operationError != nil {
			return nil, operationError
		}
		if session.InstanceID != input.WorkspaceID {
			return nil, fmt.Errorf("memory retrieval session belongs to another workspace")
		}
	}
	configuration, operationError := plugin.requestConfiguration(operationContext, input.WorkspaceID)
	if operationError != nil {
		return nil, operationError
	}
	if !configuration.Enabled || plugin.database == nil || plugin.services.Embeddings == nil {
		return nil, harness.ErrWorkspaceMemoryUnavailable
	}
	input.Query = strings.TrimSpace(input.Query)
	if input.Query == "" || len(input.Query) > 2048 {
		return nil, fmt.Errorf("memory query must contain 1 to 2048 UTF-8 bytes")
	}
	if input.Compression != "low" && input.Compression != "medium" && input.Compression != "high" {
		return nil, fmt.Errorf("memory compression %q is not supported", input.Compression)
	}
	if input.Limit < 1 || input.Limit > 20 || input.MaxBytes < 1 || input.MaxBytes > 32768 {
		return nil, fmt.Errorf("memory retrieval limits are outside supported bounds")
	}
	categories, operationError := normalizeCategories(input.Categories)
	if operationError != nil {
		return nil, operationError
	}
	started := time.Now()
	chunks, operationError := plugin.embedQueryText(operationContext, input.WorkspaceID, input.Agent, input.Query)
	if operationError != nil {
		return nil, operationError
	}
	vector, operationError := queryVector(chunks)
	if operationError != nil {
		return nil, operationError
	}
	hits, operationError := plugin.database.Search(operationContext, input.WorkspaceID, searchQuery{Text: input.Query, Vector: vector, EmbeddingModel: plugin.embeddingModel, Kinds: []string{"memory"}, Categories: categories, Limit: input.Limit})
	if operationError != nil {
		return nil, operationError
	}
	result := make([]harness.MemoryReference, 0, len(hits))
	remaining := input.MaxBytes
	for _, hit := range hits {
		record, operationError := readValue[memoryRecord](operationContext, plugin.database, input.WorkspaceID, "memory", hit.ID)
		if operationError != nil {
			return nil, operationError
		}
		if record.ID == "" || record.Version != hit.Version || (record.Status != "active" && record.Status != "consolidated") {
			continue
		}
		text := record.Text.Medium
		if input.Compression == "low" {
			text = record.Text.Low
		}
		if input.Compression == "high" {
			text = record.Text.High
		}
		if len(text) == 0 || len(text) > remaining {
			continue
		}
		remaining -= len(text)
		result = append(result, harness.MemoryReference{ID: record.ID, Version: record.Version, Categories: record.Categories, Text: text})
	}
	if operationError := plugin.database.Metric(operationContext, input.WorkspaceID, input.Agent, "memory_queries", 1, time.Since(started).Milliseconds()); operationError != nil {
		return nil, operationError
	}
	return result, nil
}
