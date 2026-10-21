package watcher

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func newID() string { return rand.Text() }
func (plugin *Plugin) runWorker() {
	defer plugin.workers.Done()
	for {
		select {
		case <-plugin.operationContext.Done():
			return
		case <-plugin.changed:
		}
		for {
			plugin.mutex.Lock()
			var value observation
			var key string
			for index, candidate := range plugin.order {
				observation, present := plugin.inbox[candidate]
				if !present {
					continue
				}
				if plugin.active[candidate] != nil {
					continue
				}
				key = candidate
				value = observation
				delete(plugin.inbox, candidate)
				plugin.order = append(plugin.order[:index], plugin.order[index+1:]...)
				break
			}
			if key == "" {
				plugin.mutex.Unlock()
				break
			}
			operationContext, cancel := context.WithCancel(plugin.operationContext)
			group := &activeGroup{workspaceID: value.Session.InstanceID, sessionID: value.Session.ID, cancel: cancel, done: make(chan struct{}), runs: map[string]bool{}}
			plugin.active[key] = group
			if len(plugin.inbox) > 0 {
				select {
				case plugin.changed <- struct{}{}:
				default:
				}
			}
			plugin.mutex.Unlock()
			plugin.reviewGroup(operationContext, value, group)
			cancel()
			plugin.mutex.Lock()
			delete(plugin.active, key)
			close(group.done)
			if len(plugin.inbox) > 0 {
				select {
				case plugin.changed <- struct{}{}:
				default:
				}
			}
			plugin.mutex.Unlock()
		}
	}
}

func permissionMatches(value definition, source harness.ReviewSource, request *atom.PermissionRequest) bool {
	if request == nil {
		return false
	}
	if len(value.PermissionTools) > 0 && (request.ToolCall == nil || !slices.Contains(value.PermissionTools, request.ToolCall.Name)) {
		return false
	}
	if !value.OutsideWorkspaceOnly {
		return true
	}
	target := request.Target
	if request.ToolCall != nil {
		target = strings.TrimPrefix(target, request.ToolCall.Name+":")
	}
	paths := []string{target}
	if strings.HasPrefix(target, "[") {
		if json.Unmarshal([]byte(target), &paths) != nil {
			return false
		}
	}
	root, operationError := harness.ResolvePath(source.Workspace.Workspace)
	if operationError != nil {
		return false
	}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			continue
		}
		relative, operationError := filepath.Rel(root, path)
		if operationError == nil && (relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
			return true
		}
	}
	return false
}

func selectDefinitions(state *sessionState, configuration configuration, source harness.ReviewSource, event observation) []definition {
	if state.UserID != source.Fence.UserMessageID {
		state.UserID = source.Fence.UserMessageID
		state.WaitingForUser = false
		state.WaitingUserID = ""
		for identifier, progress := range state.Schedules {
			progress.Continuations = 0
			progress.LimitReported = false
			state.Schedules[identifier] = progress
		}
	}
	if state.WaitingForUser && state.WaitingUserID == source.Fence.UserMessageID {
		return nil
	}
	var selected []definition
	for _, value := range configuration.Watchers {
		if !value.Enabled || !slices.Contains(value.Triggers, event.Trigger) {
			continue
		}
		if event.Trigger == "permission_request" && !permissionMatches(value, source, event.Permission) {
			continue
		}
		progress := state.Schedules[value.ID]
		signature := definitionSignature(value)
		if progress.Signature != "" && progress.Signature != signature {
			progress.Continuations = 0
			progress.LimitReported = false
		}
		if event.Trigger == "context_growth" {
			if progress.Next == 0 {
				progress.Next = value.IntervalTokens
				progress.Signature = signature
				progress.Model = event.Model
				progress.GrowthBasis = contextGrowthBasis
			}
			removed := false
			for _, identifier := range progress.Visible {
				if !slices.Contains(event.Visible, identifier) {
					removed = true
					break
				}
			}
			if removed || progress.Signature != signature || progress.Model != event.Model || progress.GrowthBasis != contextGrowthBasis {
				progress.Next = event.Tokens + value.IntervalTokens
				progress.HighWater = event.Tokens
				progress.Signature = signature
				progress.Model = event.Model
				progress.GrowthBasis = contextGrowthBasis
			}
			progress.Visible = slices.Clone(event.Visible)
			progress.HighWater = max(progress.HighWater, event.Tokens)
			if event.Tokens < progress.Next {
				state.Schedules[value.ID] = progress
				continue
			}
			progress.Next = (event.Tokens/value.IntervalTokens + 1) * value.IntervalTokens
		}
		if event.Trigger != "permission_request" && progress.Continuations >= value.MaxContinuations {
			state.Schedules[value.ID] = progress
			continue
		}
		state.Schedules[value.ID] = progress
		progress.Signature = signature
		state.Schedules[value.ID] = progress
		selected = append(selected, value)
	}
	return selected
}

func (plugin *Plugin) reviewGroup(operationContext context.Context, event observation, group *activeGroup) {
	configuration, _, operationError := plugin.loadConfiguration(operationContext, event.Session.InstanceID)
	if operationError != nil {
		plugin.recordError(event.Session.InstanceID, operationError)
		return
	}
	if !configuration.Enabled {
		return
	}
	if !slices.ContainsFunc(configuration.Watchers, func(value definition) bool { return value.Enabled && slices.Contains(value.Triggers, event.Trigger) }) {
		return
	}
	if event.Permission != nil {
		request, readError := plugin.services.Sessions.Permission(operationContext, event.Session.ID, event.Permission.ID)
		if readError != nil {
			plugin.recordError(event.Session.InstanceID, readError)
			return
		}
		if request == nil {
			return
		}
		event.Permission = request
	}
	maximum := 32768
	includeReasoning := false
	for _, value := range configuration.Watchers {
		if value.Enabled && slices.Contains(value.Triggers, event.Trigger) {
			maximum = max(maximum, templateMaximumTokens(value))
			includeReasoning = includeReasoning || value.IncludeReasoning
		}
	}
	source, operationError := plugin.services.Sessions.Source(operationContext, event.Session.ID, min(4<<20, max(4096, maximum*6)), includeReasoning)
	if operationError != nil {
		plugin.recordError(event.Session.InstanceID, operationError)
		return
	}
	if event.Fence != nil && (source.Fence.Control != event.Fence.Control || source.Fence.UserMessageID != event.Fence.UserMessageID || source.Fence.HistoryRevision != event.Fence.HistoryRevision || source.Fence.TaskRevision != event.Fence.TaskRevision) {
		return
	}
	var selected, limited []definition
	operationError = plugin.database.UpdateSession(operationContext, source.Workspace.ID, string(source.Session.ID), func(state *sessionState) error {
		selected = selectDefinitions(state, configuration, source, event)
		if event.Trigger != "permission_request" && !state.WaitingForUser {
			for _, value := range configuration.Watchers {
				progress := state.Schedules[value.ID]
				if value.Enabled && slices.Contains(value.Triggers, event.Trigger) && progress.Continuations >= value.MaxContinuations && !progress.LimitReported {
					limited = append(limited, value)
					progress.LimitReported = true
					state.Schedules[value.ID] = progress
				}
			}
		}
		return nil
	})
	if operationError != nil {
		plugin.recordError(source.Workspace.ID, operationError)
		return
	}
	for _, value := range limited {
		anchor := ""
		if len(source.Messages) > 0 {
			anchor = source.Messages[len(source.Messages)-1].ID
		}
		if recordError := plugin.storeRun(operationContext, runRecord{ID: newID(), WorkspaceID: source.Workspace.ID, SessionID: source.Session.ID, DefinitionID: value.ID, Name: value.Name, Trigger: event.Trigger, Status: "limited", Error: fmt.Sprintf("automatic intervention limit %d reached", value.MaxContinuations), CreatedAt: time.Now(), FinishedAt: time.Now(), AnchorID: anchor, AnchorSequence: source.HeadSequence}); recordError != nil {
			plugin.recordError(source.Workspace.ID, recordError)
		}
	}
	if len(selected) == 0 {
		return
	}
	updates, release, operationError := plugin.services.Sessions.Observe(operationContext, source.Session)
	if operationError != nil {
		plugin.recordError(source.Workspace.ID, operationError)
		return
	}
	defer release()
	monitorContext, stopMonitor := context.WithCancel(operationContext)
	defer stopMonitor()
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		for {
			select {
			case <-monitorContext.Done():
				return
			case <-updates:
				current, validationError := plugin.services.Sessions.Current(monitorContext, source)
				if validationError != nil || !current {
					group.cancel()
					return
				}
			}
		}
	}()
	defer func() { stopMonitor(); <-monitorDone }()
	var runs []runRecord
	allContinue := true
	needsUser := false
	var corrections []map[string]string
	for _, value := range selected {
		if operationContext.Err() != nil {
			break
		}
		run := plugin.runReview(operationContext, source, event, value, group)
		runs = append(runs, run)
		if run.Status != "reviewed" || run.Decision != "continue" {
			allContinue = false
		}
		if run.Status == "reviewed" && run.Decision == "interfere" {
			corrections = append(corrections, map[string]string{"id": value.ID, "name": value.Name, "message": run.Message})
			needsUser = needsUser || (run.ContinueChat != nil && !*run.ContinueChat)
		}
	}
	if operationContext.Err() != nil {
		plugin.finishGroup(runs, "cancelled", operationContext.Err())
		return
	}
	for _, value := range selected {
		valid, validationError := plugin.definitionCurrent(operationContext, source.Workspace.ID, value)
		if validationError != nil || !valid {
			plugin.finishGroup(runs, "stale", validationError)
			return
		}
	}
	current, operationError := plugin.services.Sessions.Current(operationContext, source)
	if operationError != nil || !current {
		plugin.finishGroup(runs, "stale", operationError)
		return
	}
	var applied bool
	if needsUser {
		if event.Permission != nil {
			var pending *atom.PermissionRequest
			pending, operationError = plugin.services.Sessions.Permission(operationContext, source.Session.ID, event.Permission.ID)
			if operationError == nil && pending == nil {
				operationError = harness.ErrReviewStale
			}
		}
		if operationError == nil {
			operationError = plugin.saveUserFeedback(operationContext, source, runs, corrections, event.Trigger != "permission_request")
			applied = operationError == nil
		}
	} else if event.Trigger == "permission_request" {
		if len(corrections) > 0 {
			applied, operationError = plugin.services.Sessions.ResolvePermission(operationContext, source, event.Permission.ID, false, "watcher", interventionText(runs, corrections))
		} else if allContinue {
			applied, operationError = plugin.services.Sessions.ResolvePermission(operationContext, source, event.Permission.ID, true, "watcher", "")
		} else {
			plugin.finishGroup(runs, "not_applied", fmt.Errorf("permission review did not complete"))
			return
		}
	} else if len(corrections) > 0 {
		if event.Trigger != "permission_request" {
			operationError = plugin.database.UpdateSession(operationContext, source.Workspace.ID, string(source.Session.ID), func(state *sessionState) error {
				if state.UserID != source.Fence.UserMessageID {
					return harness.ErrReviewStale
				}
				intervening := map[string]bool{}
				for _, correction := range corrections {
					intervening[correction["id"]] = true
				}
				for _, value := range selected {
					if !intervening[value.ID] {
						continue
					}
					progress := state.Schedules[value.ID]
					if progress.Continuations >= value.MaxContinuations {
						return fmt.Errorf("watcher continuation limit reached")
					}
				}
				for _, value := range selected {
					if !intervening[value.ID] {
						continue
					}
					progress := state.Schedules[value.ID]
					progress.Continuations++
					state.Schedules[value.ID] = progress
				}
				return nil
			})
			if operationError != nil {
				plugin.finishGroup(runs, "limited", operationError)
				return
			}
		}
		applied, operationError = plugin.services.Sessions.SubmitContinuation(operationContext, source, "watcher-"+newID(), interventionText(runs, corrections))
	} else {
		applied = true
	}
	status := "completed"
	if len(corrections) > 0 {
		status = "intervened"
	}
	if needsUser {
		status = "user_input_requested"
	}
	if operationError != nil || !applied {
		status = "stale"
		if operationError != nil && !errors.Is(operationError, harness.ErrReviewStale) {
			status = "error"
		}
	}
	plugin.finishGroup(runs, status, operationError)
}

func (plugin *Plugin) runReview(operationContext context.Context, source harness.ReviewSource, event observation, value definition, group *activeGroup) runRecord {
	run := runRecord{ID: newID(), WorkspaceID: source.Workspace.ID, SessionID: source.Session.ID, DefinitionID: value.ID, Name: value.Name, Trigger: event.Trigger, Status: "running", Model: value.Model, CreatedAt: time.Now(), AnchorSequence: source.HeadSequence}
	if len(source.Messages) > 0 {
		run.AnchorID = source.Messages[len(source.Messages)-1].ID
	}
	plugin.mutex.Lock()
	group.runs[run.ID] = true
	plugin.mutex.Unlock()
	prompt, budget, truncated, operationError := plugin.renderInput(operationContext, value, source, event)
	run.InputPreview = boundedText(prompt, 16384)
	run.InputTruncated = truncated || len(prompt) > len(run.InputPreview)
	run.InputTokens = budget.InputTokens
	run.Estimated = budget.Estimated
	if operationError == nil {
		operationError = plugin.storeRun(operationContext, run)
	}
	if operationError == nil {
		workerContext, cancel := context.WithTimeout(operationContext, time.Duration(value.TimeoutMS)*time.Millisecond)
		result, workerError := plugin.services.Sessions.RunSession(workerContext, harness.WorkerSessionRequest{WorkspaceID: source.Workspace.ID, SourceSessionID: source.Session.ID, Agent: "watcher." + value.ID, RunID: run.ID, Model: value.Model, ReasoningEffort: value.ReasoningEffort, Prompt: prompt, Tools: value.Tools, MaxRounds: value.MaxRounds, MaxOutputTokens: value.MaxOutputTokens, Started: func(identifier atom.SessionID) error {
			run.WorkerSessionID = identifier
			return plugin.storeRun(workerContext, run)
		}})
		cancel()
		operationError = workerError
		run.WorkerSessionID = result.SessionID
		run.Decision, run.Message, run.Reason = result.Decision, result.Message, result.Reason
		run.ContinueChat = result.ContinueChat
	}
	run.FinishedAt = time.Now()
	run.DurationMS = time.Since(run.CreatedAt).Milliseconds()
	run.Status = "reviewed"
	if operationError != nil {
		run.Status = "error"
		if errors.Is(operationError, context.Canceled) {
			run.Status = "cancelled"
		}
		run.Error = operationError.Error()
	}
	finishContext, cancel := context.WithTimeout(context.WithoutCancel(operationContext), 5*time.Second)
	defer cancel()
	if saveError := plugin.storeRun(finishContext, run); saveError != nil {
		plugin.recordError(run.WorkspaceID, saveError)
	}
	return run
}
func boundedText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	for limit > 0 && (text[limit]&0xc0) == 0x80 {
		limit--
	}
	return strings.Clone(text[:limit])
}
func interventionText(runs []runRecord, corrections []map[string]string) string {
	var identifiers []string
	for _, run := range runs {
		identifiers = append(identifiers, run.ID)
	}
	data, _ := json.Marshal(map[string]any{"type": "watcher.intervention", "run_ids": identifiers, "notes": corrections})
	return "[watcher]\n" + string(data)
}
func (plugin *Plugin) definitionCurrent(operationContext context.Context, workspaceID string, value definition) (bool, error) {
	configuration, _, operationError := plugin.loadConfiguration(operationContext, workspaceID)
	if operationError != nil {
		return false, operationError
	}
	if !configuration.Enabled {
		return false, nil
	}
	for _, current := range configuration.Watchers {
		if current.ID == value.ID {
			return current.Enabled && definitionSignature(current) == definitionSignature(value), nil
		}
	}
	return false, nil
}
func (plugin *Plugin) storeRun(operationContext context.Context, run runRecord) error {
	if operationError := plugin.database.SaveRun(operationContext, run); operationError != nil {
		return operationError
	}
	payload, _ := json.Marshal(map[string]string{"plugin": pluginName, "run_id": run.ID, "status": run.Status})
	return plugin.services.Events.Send(operationContext, atom.Event{InstanceID: run.WorkspaceID, SessionID: run.SessionID, Name: "plugin.activity", Payload: payload, Time: time.Now()})
}
func (plugin *Plugin) finishGroup(runs []runRecord, status string, operationError error) {
	for _, run := range runs {
		if run.Status != "reviewed" {
			continue
		}
		run.Status = status
		if status == "user_input_requested" {
			if run.Decision == "continue" {
				run.Status = "completed"
			} else if run.ContinueChat == nil || *run.ContinueChat {
				run.Status = "deferred_for_user"
			}
		}
		if operationError != nil {
			run.Error = operationError.Error()
		}
		operationContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		saveError := plugin.storeRun(operationContext, run)
		cancel()
		if saveError != nil {
			plugin.recordError(run.WorkspaceID, saveError)
		}
	}
}
