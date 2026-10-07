package contextplugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type preparedMemory struct {
	Proposal memoryProposal
	Vectors  []vectorChunk
}

func (plugin *Plugin) publishProposals(operationContext context.Context, workspaceID string, job memoryJob) error {
	prepared := []preparedMemory{}
	seen := map[string]bool{}
	for _, proposal := range job.Proposals {
		key := strings.ToLower(strings.TrimSpace(proposal.Title)) + "\x00" + proposal.Text.High
		if seen[key] {
			continue
		}
		seen[key] = true
		vectors, operationError := plugin.embedText(operationContext, workspaceID, job.Agent, proposal.Title+"\n"+proposal.Text.High+"\n"+proposal.Text.Medium+"\n"+proposal.Text.Low)
		if operationError != nil {
			return operationError
		}
		prepared = append(prepared, preparedMemory{Proposal: proposal, Vectors: vectors})
	}
	return plugin.database.Transact(operationContext, workspaceID, func(database transaction) error {
		stored, operationError := validateJobLease(database, job)
		if operationError != nil {
			return operationError
		}
		for identifier, version := range stored.CandidateVersions {
			record, operationError := readTransactionValue[memoryRecord](database, "memory", identifier)
			if operationError != nil {
				return operationError
			}
			if record.Version != version {
				return fmt.Errorf("related memory changed during extraction")
			}
		}
		all, operationError := readTransactionValues[memoryRecord](database, "memory")
		if operationError != nil {
			return operationError
		}
		byHigh := map[string]memoryRecord{}
		for _, record := range all {
			if record.Status == "active" || record.Status == "consolidated" {
				byHigh[strings.ToLower(strings.TrimSpace(record.Text.High))] = record
			}
		}
		for index, item := range prepared {
			proposal := item.Proposal
			var sources []int64
			for _, evidence := range proposal.Evidence {
				sources = append(sources, evidence.SourceID)
				if evidence.ApprovalID != 0 {
					sources = append(sources, evidence.ApprovalID)
				}
			}
			sources = uniqueSourceIDs(sources)
			for _, identifier := range sources {
				source, operationError := readTransactionValue[source](database, "source", strconv.FormatInt(identifier, 10))
				if operationError != nil {
					return operationError
				}
				if source.ID == 0 || source.Deleted {
					return fmt.Errorf("memory evidence changed before publication")
				}
			}
			if existing, found := byHigh[strings.ToLower(strings.TrimSpace(proposal.Text.High))]; found && len(proposal.Supersedes) == 0 && sameSummary(existing.Text, proposal.Text) {
				stored.ResultIDs = append(stored.ResultIDs, existing.ID)
				continue
			}
			identity := sha256.Sum256([]byte(job.ID + ":" + strconv.Itoa(index)))
			record := memoryRecord{ID: hex.EncodeToString(identity[:]), Version: 1, Title: proposal.Title, Categories: proposal.Categories, Text: proposal.Text, Kind: proposal.Kind, Status: "active", SourceIDs: sources, Important: proposal.Important, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
			if len(proposal.Supersedes) > 0 {
				identifiers := append([]string(nil), proposal.Supersedes...)
				sort.Strings(identifiers)
				previous, operationError := readTransactionValue[memoryRecord](database, "memory", identifiers[0])
				if operationError != nil {
					return operationError
				}
				if previous.ID == "" {
					return fmt.Errorf("replacement memory is no longer available")
				}
				record.ID = previous.ID
				record.Version = previous.Version + 1
				record.CreatedAt = previous.CreatedAt
				for _, identifier := range identifiers {
					previous, operationError := readTransactionValue[memoryRecord](database, "memory", identifier)
					if operationError != nil {
						return operationError
					}
					if previous.Kind == "idea" {
						if operationError := activateSupportedChildren(database, previous.Children); operationError != nil {
							return operationError
						}
					}
					previous.Status = "superseded"
					if operationError := database.Put("memory", identifier, previous); operationError != nil {
						return operationError
					}
					if operationError := database.MarkVectors("memory", identifier, false, true); operationError != nil {
						return operationError
					}
					if operationError := invalidateIdeas(database, all, identifier, record.ID); operationError != nil {
						return operationError
					}
				}
			}
			if operationError := database.Put("memory", record.ID, record); operationError != nil {
				return operationError
			}
			if operationError := database.Put("memory_version", record.ID+":"+strconv.Itoa(record.Version), record); operationError != nil {
				return operationError
			}
			if operationError := database.ReplaceVectors("memory", record.ID, record.Version, plugin.embeddingModel, record.Categories, item.Vectors, false, false); operationError != nil {
				return operationError
			}
			stored.ResultIDs = append(stored.ResultIDs, record.ID)
		}
		stored.Status = "ready"
		stored.Error = ""
		stored.Lease = ""
		stored.LeaseUntil = time.Time{}
		if stored.Kind != "reduce" {
			stored.Status = "applied"
		}
		if operationError := database.AddMetric(job.Agent, "prepared_jobs", 1, 0); operationError != nil {
			return operationError
		}
		if operationError := database.AddMetric(job.Agent, "published_memories", int64(len(prepared)), 0); operationError != nil {
			return operationError
		}
		return database.Put("job", stored.ID, stored)
	})
}
