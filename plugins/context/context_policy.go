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

var errContextChanged = errors.New("context view changed before commit")

type preparedReduction struct {
	View         sessionView
	Jobs         []memoryJob
	Memories     []memoryRecord
	RemovedCount int
	Pending      bool
	Failure      string
}

func (plugin *Plugin) PrepareContext(operationContext context.Context, input harness.ContextRequest) (harness.ContextSelection, error) {
	configuration, operationError := plugin.requestConfiguration(operationContext, input.Session.InstanceID)
	if operationError != nil {
		return harness.ContextSelection{}, operationError
	}
	selection := harness.ContextSelection{Request: input.Request, Managed: configuration.Enabled}
	if configuration.Enabled && input.Budget.InputLimit > 0 {
		selection.InputCeiling = (input.Budget.InputLimit*configuration.HardPercent - 1) / 100
	}
	if plugin.database == nil {
		if configuration.Enabled {
			return selection, plugin.unavailable
		}
		return selection, nil
	}
	if !configuration.Enabled {
		view, operationError := readValue[sessionView](operationContext, plugin.database, input.Session.InstanceID, "view", string(input.Session.ID))
		if operationError != nil {
			return selection, operationError
		}
		if view.Initialized {
			selection.Request = projectContext(input.Request, view)
		}
		return selection, nil
	}
	if input.Budget.InputLimit <= 0 {
		return selection, fmt.Errorf("context plugin requires a known input budget for model %q", input.Model.ID)
	}
	view, _, operationError := plugin.archiveRequest(operationContext, input.Session.InstanceID, input.Session.ID, input.Request.Messages)
	if operationError != nil {
		return selection, operationError
	}
	view.Groups, view.Protected = contextGroups(input.Request.Messages, view)
	view.Available = nil
	for _, group := range view.Groups {
		view.Available = append(view.Available, group...)
	}
	if !view.Initialized {
		var records []memoryRecord
		operationError = plugin.database.Transact(operationContext, input.Session.InstanceID, func(database transaction) error {
			var readError error
			records, readError = readTransactionValues[memoryRecord](database, "memory")
			return readError
		})
		if operationError != nil {
			return selection, operationError
		}
		newSession := true
		for _, message := range input.Request.Messages {
			if message.Role == atom.RoleAssistant {
				newSession = false
				break
			}
		}
		if newSession {
			view.Snapshot, operationError = selectSnapshot(operationContext, input, configuration, view, records, false)
			if operationError != nil {
				return selection, operationError
			}
		}
		view.Initialized = true
	}
	if operationError := plugin.savePreparedView(operationContext, input.Session.InstanceID, view); operationError != nil {
		return selection, operationError
	}
	projected := projectContext(input.Request, view)
	budget, operationError := input.Measure(operationContext, projected)
	if operationError != nil {
		return selection, operationError
	}
	forced := budget.InputTokens*100 >= budget.InputLimit*configuration.HardPercent
	view.InputTokens, view.InputLimit = budget.InputTokens, budget.InputLimit
	if view.WrapRequested || forced {
		deadline := time.NewTimer(time.Duration(configuration.JobTimeoutMilliseconds) * time.Millisecond)
		defer deadline.Stop()
		for {
			prepared, prepareError := plugin.prepareReduction(operationContext, input.Session.InstanceID, view)
			if prepareError != nil {
				return selection, prepareError
			}
			candidate := prepared.View
			if prepared.RemovedCount > 0 {
				candidate.Snapshot, operationError = selectSnapshot(operationContext, input, configuration, view, prepared.Memories, true)
				if operationError != nil {
					return selection, operationError
				}
				projected = projectContext(input.Request, candidate)
				budget, operationError = input.Measure(operationContext, projected)
				if operationError != nil {
					return selection, operationError
				}
				candidate.InputTokens, candidate.InputLimit = budget.InputTokens, budget.InputLimit
			}
			if !forced || budget.InputTokens*100 < budget.InputLimit*configuration.TargetPercent {
				if operationError := plugin.commitReduction(operationContext, input.Session.InstanceID, view, prepared, candidate); operationError != nil {
					return selection, operationError
				}
				view = candidate
				break
			}
			queued, queueError := plugin.queueForcedReduction(operationContext, input.Session.InstanceID, view, prepared)
			if queueError != nil {
				return selection, queueError
			}
			if !queued && !prepared.Pending {
				return selection, fmt.Errorf("context uses %d of %d input tokens; no further prepared removable groups; worker error: %s", budget.InputTokens, budget.InputLimit, prepared.Failure)
			}
			plugin.signalWork()
			pause := time.NewTimer(100 * time.Millisecond)
			select {
			case <-operationContext.Done():
				pause.Stop()
				return selection, operationContext.Err()
			case <-deadline.C:
				pause.Stop()
				return selection, fmt.Errorf("forced context checkpoint is waiting for memory preparation")
			case <-pause.C:
			}
		}
		view, operationError = readValue[sessionView](operationContext, plugin.database, input.Session.InstanceID, "view", string(input.Session.ID))
		if operationError != nil {
			return selection, operationError
		}
		projected = projectContext(input.Request, view)
		budget, operationError = input.Measure(operationContext, projected)
		if operationError != nil {
			return selection, operationError
		}
	}
	if budget.InputTokens*100 >= budget.InputLimit*configuration.HardPercent {
		return selection, fmt.Errorf("context remains at or above the %d%% request ceiling", configuration.HardPercent)
	}
	level := 0
	if budget.InputTokens*100 >= budget.InputLimit*configuration.UrgentPercent {
		level = 2
	} else if budget.InputTokens*100 >= budget.InputLimit*configuration.ReviewPercent {
		level = 1
	}
	if level > view.NoticeLevel {
		kind := "review"
		if level == 2 {
			kind = "urgent"
		}
		data, _ := json.Marshal(map[string]any{"notice": kind, "input_tokens": budget.InputTokens, "input_budget": budget.InputLimit, "estimated": budget.Estimated})
		projected.Messages = append(projected.Messages, atom.Message{SessionID: input.Session.ID, Role: atom.RoleRuntime, Ephemeral: true, Content: []atom.Content{{Type: atom.Text, Text: "Context budget: " + string(data)}}})
		withNotice, operationError := input.Measure(operationContext, projected)
		if operationError != nil {
			return selection, operationError
		}
		if withNotice.InputTokens*100 >= withNotice.InputLimit*configuration.HardPercent {
			// The notice is optional tail data; it cannot push an otherwise
			// permitted request over the hard ceiling it describes.
			projected.Messages = projected.Messages[:len(projected.Messages)-1]
		} else {
			view.NoticeLevel = level
			budget = withNotice
		}
	}
	view.InputTokens, view.InputLimit = budget.InputTokens, budget.InputLimit
	view.Groups, view.Protected = contextGroups(input.Request.Messages, view)
	view.Available = nil
	for _, group := range view.Groups {
		view.Available = append(view.Available, group...)
	}
	if operationError := plugin.savePreparedView(operationContext, input.Session.InstanceID, view); operationError != nil {
		return selection, operationError
	}
	selection.Request = projected
	return selection, nil
}

func (plugin *Plugin) savePreparedView(operationContext context.Context, workspaceID string, view sessionView) error {
	return plugin.database.Transact(operationContext, workspaceID, func(database transaction) error {
		current, operationError := loadView(database, view.SessionID)
		if operationError != nil {
			return operationError
		}
		if current.Epoch != view.Epoch || current.Revision != view.Revision || current.Mutation != nil || current.Deleted {
			return errContextChanged
		}
		// Search promotions and a tool's pending wrap-up survive independent
		// request metadata writes. Only a committed refresh consumes promotions.
		for identifier := range current.Promote {
			view.Promote[identifier] = true
		}
		view.WrapRequested = current.WrapRequested
		if !current.Initialized && view.Initialized {
			if operationError := recordSnapshotMetrics(database, view.Snapshot); operationError != nil {
				return operationError
			}
		}
		return database.Put("view", string(view.SessionID), view)
	})
}

func (plugin *Plugin) prepareReduction(operationContext context.Context, workspaceID string, view sessionView) (preparedReduction, error) {
	prepared := preparedReduction{}
	operationError := plugin.database.Transact(operationContext, workspaceID, func(database transaction) error {
		current, operationError := loadView(database, view.SessionID)
		if operationError != nil {
			return operationError
		}
		if current.Epoch != view.Epoch || current.Revision != view.Revision || current.Deleted || current.Mutation != nil {
			return errContextChanged
		}
		prepared.View = current
		jobs, operationError := readTransactionValues[memoryJob](database, "job")
		if operationError != nil {
			return operationError
		}
		for _, job := range jobs {
			if job.SessionID != view.SessionID || job.Epoch != view.Epoch || job.Kind != "reduce" {
				continue
			}
			if job.Status == "pending" || job.Status == "running" {
				prepared.Pending = true
			}
			if job.Status == "failed" {
				prepared.Failure = job.Error
			}
			if job.Status != "ready" {
				continue
			}
			for _, identifier := range job.SourceIDs {
				if view.Protected[identifier] {
					return fmt.Errorf("prepared reduction includes protected message %d", identifier)
				}
				source, operationError := readTransactionValue[source](database, "source", strconv.FormatInt(identifier, 10))
				if operationError != nil {
					return operationError
				}
				if source.ID == 0 || !source.Indexed || source.Deleted {
					return fmt.Errorf("reduction source %d is not durably prepared", identifier)
				}
				if !prepared.View.Removed[source.MessageID] {
					prepared.View.Removed[source.MessageID] = true
					prepared.RemovedCount++
				}
			}
			prepared.Jobs = append(prepared.Jobs, job)
		}
		prepared.Memories, operationError = readTransactionValues[memoryRecord](database, "memory")
		return operationError
	})
	return prepared, operationError
}

func (plugin *Plugin) queueForcedReduction(operationContext context.Context, workspaceID string, view sessionView, prepared preparedReduction) (bool, error) {
	queued := false
	operationError := plugin.database.Transact(operationContext, workspaceID, func(database transaction) error {
		jobs, operationError := readTransactionValues[memoryJob](database, "job")
		if operationError != nil {
			return operationError
		}
		claimed := map[int64]bool{}
		for _, job := range jobs {
			if job.SessionID == view.SessionID && job.Epoch == view.Epoch && job.Kind == "reduce" {
				for _, identifier := range job.SourceIDs {
					claimed[identifier] = true
				}
			}
		}
		var selected []int64
		for _, group := range view.Groups {
			if groupContainsProtected(group, view.Protected) {
				continue
			}
			already := false
			for _, identifier := range group {
				already = already || claimed[identifier]
			}
			if already {
				continue
			}
			if len(selected)+len(group) > 128 {
				break
			}
			selected = append(selected, group...)
		}
		if len(selected) == 0 {
			return nil
		}
		_, operationError = enqueueReduction(database, view, selected, true, nil, true)
		queued = operationError == nil
		return operationError
	})
	return queued, operationError
}

func (plugin *Plugin) commitReduction(operationContext context.Context, workspaceID string, previous sessionView, prepared preparedReduction, candidate sessionView) error {
	return plugin.database.Transact(operationContext, workspaceID, func(database transaction) error {
		current, operationError := loadView(database, previous.SessionID)
		if operationError != nil {
			return operationError
		}
		if current.Epoch != previous.Epoch || current.Revision != previous.Revision || current.Deleted || current.Mutation != nil {
			return errContextChanged
		}
		current.WrapRequested = false
		if prepared.RemovedCount == 0 {
			return database.Put("view", string(previous.SessionID), current)
		}
		for _, job := range prepared.Jobs {
			stored, operationError := readTransactionValue[memoryJob](database, "job", job.ID)
			if operationError != nil {
				return operationError
			}
			if stored.Status != "ready" || stored.Epoch != previous.Epoch {
				return errContextChanged
			}
			stored.Status = "applied"
			if operationError := database.Put("job", job.ID, stored); operationError != nil {
				return operationError
			}
		}
		for _, entry := range candidate.Snapshot {
			record, operationError := readTransactionValue[memoryRecord](database, "memory", entry.MemoryID)
			if operationError != nil {
				return operationError
			}
			if record.Version != entry.Version || record.Status == "superseded" || record.Status == "deleted" || record.Status == "invalidated" {
				return errContextChanged
			}
		}
		candidate.Revision++
		candidate.NoticeLevel = 0
		candidate.WrapRequested = false
		candidate.Promote = map[string]bool{}
		if operationError := database.AddMetric("context.compactor", "checkpoints", 1, 0); operationError != nil {
			return operationError
		}
		if operationError := database.AddMetric("context.compactor", "removed_messages", int64(prepared.RemovedCount), 0); operationError != nil {
			return operationError
		}
		if operationError := database.AddMetric("context.compactor", "estimated_tokens_removed", int64(max(0, previous.InputTokens-candidate.InputTokens)), 0); operationError != nil {
			return operationError
		}
		if operationError := recordSnapshotMetrics(database, candidate.Snapshot); operationError != nil {
			return operationError
		}
		return database.Put("view", string(previous.SessionID), candidate)
	})
}

func recordSnapshotMetrics(database transaction, entries []presentation) error {
	if operationError := database.AddMetric("context.presentation", "snapshots", 1, 0); operationError != nil {
		return operationError
	}
	counts := map[compression]int64{}
	for _, entry := range entries {
		counts[entry.Level]++
	}
	for level, count := range counts {
		if operationError := database.AddMetric("context.presentation", "level_"+string(level), count, 0); operationError != nil {
			return operationError
		}
	}
	return nil
}
