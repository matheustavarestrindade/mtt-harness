package contextplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (plugin *Plugin) BeginHistoryChange(operationContext context.Context, change harness.HistoryChange) (func(context.Context, bool) error, error) {
	noop := func(context.Context, bool) error { return nil }
	configuration, _, operationError := plugin.loadConfiguration(operationContext, change.WorkspaceID)
	if operationError != nil {
		return nil, operationError
	}
	if plugin.database == nil {
		if configuration.Enabled {
			return nil, plugin.unavailable
		}
		return noop, nil
	}
	identifier := newIdentifier()
	affected := []atom.SessionID{}
	cancelJobs := []string{}
	operationError = plugin.database.Transact(operationContext, change.WorkspaceID, func(database transaction) error {
		for _, snapshot := range change.Snapshots {
			view, operationError := loadView(database, snapshot.Session.ID)
			if operationError != nil {
				return operationError
			}
			if !configuration.Enabled && !view.Initialized && len(view.Sources) == 0 {
				continue
			}
			if view.Deleted || view.Mutation != nil {
				return fmt.Errorf("conversation already has a pending history change")
			}
			if _, operationError := archiveMessages(database, &view, snapshot.Messages); operationError != nil {
				return operationError
			}
			var removedIDs []int64
			pastAnchor := change.Kind == harness.HistoryDelete
			var anchorTime time.Time
			for _, message := range snapshot.Messages {
				if pastAnchor {
					if sourceID := view.Sources[message.ID]; sourceID != 0 {
						removedIDs = append(removedIDs, sourceID)
					}
				}
				if message.ID == change.KeepMessageID {
					pastAnchor = true
					anchorTime = message.CreatedAt
				}
			}
			allSources, operationError := readTransactionValues[source](database, "source")
			if operationError != nil {
				return operationError
			}
			for _, source := range allSources {
				if source.SessionID == snapshot.Session.ID && (change.Kind == harness.HistoryDelete || source.MessageID != "" && source.CreatedAt.After(anchorTime) && view.Sources[source.MessageID] == 0) {
					removedIDs = append(removedIDs, source.ID)
				}
			}
			view.Epoch++
			view.Mutation = &historyMutation{ID: identifier, Owner: plugin.processID, Kind: string(change.Kind), KeepMessageID: change.KeepMessageID, SourceIDs: uniqueSourceIDs(removedIDs)}
			if operationError := database.Put("view", string(view.SessionID), view); operationError != nil {
				return operationError
			}
			affected = append(affected, view.SessionID)
			jobs, operationError := readTransactionValues[memoryJob](database, "job")
			if operationError != nil {
				return operationError
			}
			for _, job := range jobs {
				if job.SessionID != view.SessionID || job.Status == "applied" || job.Status == "cancelled" {
					continue
				}
				job.Status = "cancelled"
				job.Error = "source history changed"
				cancelJobs = append(cancelJobs, job.ID)
				if operationError := database.Put("job", job.ID, job); operationError != nil {
					return operationError
				}
			}
		}
		return nil
	})
	if operationError != nil {
		return nil, operationError
	}
	plugin.mutex.Lock()
	if scope := plugin.scopes[change.WorkspaceID]; scope != nil {
		for _, identifier := range cancelJobs {
			if cancel := scope.jobs[identifier]; cancel != nil {
				cancel()
			}
		}
	}
	plugin.mutex.Unlock()
	return func(completionContext context.Context, committed bool) error {
		return plugin.finishHistoryChange(completionContext, change.WorkspaceID, affected, identifier, committed)
	}, nil
}

func (plugin *Plugin) finishHistoryChange(operationContext context.Context, workspaceID string, sessions []atom.SessionID, identifier string, committed bool) error {
	operationError := plugin.database.Transact(operationContext, workspaceID, func(database transaction) error {
		deletedSources := map[int64]bool{}
		for _, sessionID := range sessions {
			view, operationError := loadView(database, sessionID)
			if operationError != nil {
				return operationError
			}
			if view.Mutation == nil {
				continue
			}
			if view.Mutation.ID != identifier {
				return fmt.Errorf("history completion does not match its prepared change")
			}
			mutation := view.Mutation
			if committed {
				for _, sourceID := range mutation.SourceIDs {
					entry, operationError := readTransactionValue[source](database, "source", strconv.FormatInt(sourceID, 10))
					if operationError != nil {
						return operationError
					}
					if entry.ID == 0 {
						continue
					}
					entry.Deleted = true
					deletedSources[sourceID] = true
					if operationError := database.Put("source", strconv.FormatInt(sourceID, 10), entry); operationError != nil {
						return operationError
					}
					if operationError := database.MarkVectors("source", strconv.FormatInt(sourceID, 10), true, false); operationError != nil {
						return operationError
					}
				}
				view.Deleted = mutation.Kind == string(harness.HistoryDelete)
				view.Initialized = false
				view.Removed = map[string]bool{}
				view.Snapshot = nil
				view.Promote = map[string]bool{}
				view.Available = nil
				view.Groups = nil
				view.Protected = map[int64]bool{}
				view.WrapRequested = false
				view.Revision++
				if len(mutation.SourceIDs) > 0 {
					job := memoryJob{ID: "archive-" + identifier + "-" + string(sessionID), SessionID: sessionID, Epoch: view.Epoch, Kind: "archive", Agent: "context.archive", Status: "pending", Priority: 30, SourceIDs: mutation.SourceIDs, CreatedAt: time.Now().UTC()}
					if operationError := database.Put("job", job.ID, job); operationError != nil {
						return operationError
					}
				}
			}
			view.Mutation = nil
			if operationError := database.Put("view", string(sessionID), view); operationError != nil {
				return operationError
			}
		}
		if !committed {
			return nil
		}
		memories, operationError := readTransactionValues[memoryRecord](database, "memory")
		if operationError != nil {
			return operationError
		}
		for _, record := range memories {
			removed := false
			for _, sourceID := range record.SourceIDs {
				removed = removed || deletedSources[sourceID]
			}
			if !removed {
				continue
			}
			record.Status = "deleted"
			if operationError := database.Put("memory", record.ID, record); operationError != nil {
				return operationError
			}
			if operationError := database.MarkVectors("memory", record.ID, true, record.Status == "superseded"); operationError != nil {
				return operationError
			}
		}
		return nil
	})
	if operationError == nil {
		plugin.signalWork()
	}
	return operationError
}

// recoverHistoryChanges reconciles only a previous process's interrupted hook.
// A hook owned by this process can still be inside core storage and is never
// inferred complete from an intermediate read.
func (plugin *Plugin) recoverHistoryChanges(operationContext context.Context, workspaceID string) error {
	after := ""
	for {
		documents, operationError := plugin.database.List(operationContext, workspaceID, "view", after, 100)
		if operationError != nil {
			return operationError
		}
		if len(documents) == 0 {
			return nil
		}
		for _, document := range documents {
			var view sessionView
			if operationError := json.Unmarshal(document, &view); operationError != nil {
				return operationError
			}
			after = string(view.SessionID)
			if view.Mutation == nil || view.Mutation.Owner == plugin.processID {
				continue
			}
			_, sessionError := plugin.services.Conversations.Get(operationContext, view.SessionID)
			committed := errors.Is(sessionError, harness.ErrConversationUnavailable)
			if sessionError != nil && !committed {
				return sessionError
			}
			if !committed && view.Mutation.Kind == string(harness.HistoryRevert) {
				messages, operationError := plugin.services.Conversations.Messages(operationContext, view.SessionID)
				if operationError != nil {
					return operationError
				}
				live := map[string]bool{}
				for _, message := range messages {
					live[message.ID] = true
				}
				committed = true
				for _, sourceID := range view.Mutation.SourceIDs {
					entry, operationError := readValue[source](operationContext, plugin.database, workspaceID, "source", strconv.FormatInt(sourceID, 10))
					if operationError != nil {
						return operationError
					}
					if live[entry.MessageID] {
						committed = false
						break
					}
				}
			}
			if operationError := plugin.finishHistoryChange(operationContext, workspaceID, []atom.SessionID{view.SessionID}, view.Mutation.ID, committed); operationError != nil {
				return operationError
			}
		}
		if len(documents) < 100 {
			return nil
		}
	}
}
