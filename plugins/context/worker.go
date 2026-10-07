package contextplugin

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (plugin *Plugin) executeJob(operationContext context.Context, workspaceID string, job memoryJob, configuration configuration) error {
	if job.Kind == "historian" || job.Kind == "repair" {
		return plugin.runHistorian(operationContext, workspaceID, job, configuration)
	}
	var sources, contextSources []source
	operationError := plugin.database.Transact(operationContext, workspaceID, func(database transaction) error {
		if _, operationError := validateJobLease(database, job); operationError != nil {
			return operationError
		}
		var operationError error
		sources, operationError = readSources(database, job.SourceIDs)
		if operationError != nil {
			return operationError
		}
		contextSources, operationError = readSources(database, job.ContextIDs)
		return operationError
	})
	if operationError != nil {
		return operationError
	}
	for _, source := range sources {
		if operationError := plugin.indexSource(operationContext, workspaceID, job, source); operationError != nil {
			return operationError
		}
	}
	if job.Remember {
		if operationError := plugin.runExtraction(operationContext, workspaceID, &job, configuration, sources, contextSources); operationError != nil {
			return operationError
		}
	}
	return plugin.publishProposals(operationContext, workspaceID, job)
}

// indexSource checkpoints each tokenizer-sized chunk. A timeout or restart
// resumes at the first missing chunk rather than paying for the whole source.
func (plugin *Plugin) indexSource(operationContext context.Context, workspaceID string, job memoryJob, entry source) error {
	if entry.Indexed && entry.IndexModel == plugin.embeddingModel {
		return nil
	}
	if entry.Deleted && job.Kind != "archive" {
		return fmt.Errorf("source %d was removed from live history", entry.ID)
	}
	chunks, operationError := plugin.services.Embeddings.Split(operationContext, entry.Text)
	if operationError != nil {
		return operationError
	}
	if entry.IndexModel != plugin.embeddingModel {
		operationError = plugin.database.Transact(operationContext, workspaceID, func(database transaction) error {
			if _, operationError := validateJobLease(database, job); operationError != nil {
				return operationError
			}
			if operationError := database.ReplaceVectors("source", strconv.FormatInt(entry.ID, 10), 1, plugin.embeddingModel, job.Categories, nil, entry.Deleted, false); operationError != nil {
				return operationError
			}
			entry.IndexedChunks = 0
			entry.Indexed = false
			entry.IndexModel = plugin.embeddingModel
			return database.Put("source", strconv.FormatInt(entry.ID, 10), entry)
		})
		if operationError != nil {
			return operationError
		}
	}
	for position := entry.IndexedChunks; position < len(chunks); position++ {
		if operationError := operationContext.Err(); operationError != nil {
			return operationError
		}
		started := time.Now()
		vectors, operationError := plugin.services.Embeddings.Embed(operationContext, []string{chunks[position]})
		if operationError != nil {
			return operationError
		}
		if len(vectors) != 1 {
			return fmt.Errorf("embedding service returned %d vectors for one source chunk", len(vectors))
		}
		operationError = plugin.database.Transact(operationContext, workspaceID, func(database transaction) error {
			if _, operationError := validateJobLease(database, job); operationError != nil {
				return operationError
			}
			current, operationError := readTransactionValue[source](database, "source", strconv.FormatInt(entry.ID, 10))
			if operationError != nil {
				return operationError
			}
			if current.Deleted && job.Kind != "archive" {
				return fmt.Errorf("source history changed before embedding commit")
			}
			if operationError := database.PutSourceVector(strconv.FormatInt(entry.ID, 10), position, plugin.embeddingModel, job.Categories, vectorChunk{Text: chunks[position], Vector: vectors[0]}, current.Deleted); operationError != nil {
				return operationError
			}
			current.IndexedChunks = position + 1
			current.Indexed = position+1 == len(chunks)
			current.IndexModel = plugin.embeddingModel
			if operationError := database.Put("source", strconv.FormatInt(entry.ID, 10), current); operationError != nil {
				return operationError
			}
			stored, operationError := validateJobLease(database, job)
			if operationError != nil {
				return operationError
			}
			stored.Progress++
			return database.Put("job", job.ID, stored)
		})
		if operationError != nil {
			return operationError
		}
		if operationError := plugin.database.Metric(operationContext, workspaceID, job.Agent, "embedding_chunks", 1, time.Since(started).Milliseconds()); operationError != nil {
			return operationError
		}
	}
	return nil
}

func (plugin *Plugin) checkCostBudget(operationContext context.Context, workspaceID string, configuration configuration, modelID, effort string, messages []atom.Message) error {
	if configuration.CostLimit == 0 {
		return nil
	}
	model, operationError := plugin.services.Models.Model(operationContext, workspaceID, modelID, effort)
	if operationError != nil {
		return operationError
	}
	if model.Billing == "subscription" {
		return nil
	}
	usage := atom.Usage{Output: configuration.WorkerOutputTokens}
	for _, message := range messages {
		usage.Input += 16
		for _, content := range message.Content {
			usage.Input += len(content.Text)
		}
	}
	estimate := atom.CalculateUsageCost(model, &usage)
	if estimate == nil || estimate.Currency != configuration.CostCurrency {
		return fmt.Errorf("worker price is unavailable in cost-limit currency %s", configuration.CostCurrency)
	}
	statistics, operationError := plugin.services.Usage.Agents(operationContext, workspaceID)
	if operationError != nil {
		return operationError
	}
	spent := 0.0
	for _, agent := range statistics {
		if !strings.HasPrefix(agent.Agent, "context.") {
			continue
		}
		for _, cost := range agent.Statistics.Costs {
			if cost.Currency == configuration.CostCurrency {
				spent += cost.Value
			}
		}
	}
	if spent+estimate.Value > configuration.CostLimit {
		return fmt.Errorf("workspace context-agent cost %.6f plus request estimate %.6f exceeds limit %.6f %s", spent, estimate.Value, configuration.CostLimit, configuration.CostCurrency)
	}
	return nil
}
