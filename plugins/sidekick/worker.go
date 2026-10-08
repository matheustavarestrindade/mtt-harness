package sidekick

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (plugin *Plugin) runSessionWorker(operationContext context.Context, job *sessionWorker) {
	defer plugin.workers.Done()
	defer close(job.done)
	defer job.cancel()
	defer func() {
		plugin.mutex.Lock()
		if plugin.jobs[job.latest.session.ID] == job {
			delete(plugin.jobs, job.latest.session.ID)
		}
		plugin.signalChangedLocked()
		plugin.mutex.Unlock()
	}()
	for {
		plugin.mutex.Lock()
		observed := job.latest
		plugin.mutex.Unlock()
		if operationContext.Err() != nil || observed.state.Doing == nil {
			return
		}
		configuration, _, operationError := plugin.loadConfiguration(operationContext, observed.session.InstanceID)
		if operationError != nil {
			plugin.recordError(observed.session.InstanceID, operationError)
			return
		}
		if !configuration.Enabled {
			return
		}
		timer := time.NewTimer(time.Duration(configuration.DebounceMilliseconds) * time.Millisecond)
		select {
		case <-operationContext.Done():
			timer.Stop()
			return
		case <-job.changed:
			timer.Stop()
			continue
		case <-timer.C:
		}
		plugin.mutex.Lock()
		if job.latest.generation != observed.generation {
			plugin.mutex.Unlock()
			continue
		}
		jobContext, cancel := context.WithTimeout(operationContext, time.Duration(configuration.JobTimeoutMilliseconds)*time.Millisecond)
		job.activeCancel = cancel
		plugin.mutex.Unlock()
		operationError = plugin.acquireWorkerSlot(jobContext, observed.session.InstanceID, configuration.WorkerCount)
		if operationError == nil {
			operationError = plugin.prepareTaskHint(jobContext, observed, configuration)
			plugin.releaseWorkerSlot(observed.session.InstanceID)
		}
		cancel()
		if operationError != nil && !errors.Is(operationError, context.Canceled) && !errors.Is(operationError, errRetired) {
			plugin.recordError(observed.session.InstanceID, operationError)
		}
		plugin.mutex.Lock()
		job.activeCancel = nil
		unchanged := job.latest.generation == observed.generation
		if unchanged {
			delete(plugin.jobs, observed.session.ID)
		}
		plugin.mutex.Unlock()
		if unchanged {
			return
		}
	}
}

func (plugin *Plugin) acquireWorkerSlot(operationContext context.Context, workspaceID string, limit int) error {
	for {
		if operationError := operationContext.Err(); operationError != nil {
			return operationError
		}
		plugin.mutex.Lock()
		if plugin.closed {
			plugin.mutex.Unlock()
			return context.Canceled
		}
		scope := plugin.scopeLocked(workspaceID)
		if scope.running < limit {
			scope.running++
			plugin.mutex.Unlock()
			return nil
		}
		changed := plugin.changed
		plugin.mutex.Unlock()
		select {
		case <-operationContext.Done():
			return operationContext.Err()
		case <-changed:
		}
	}
}
func (plugin *Plugin) releaseWorkerSlot(workspaceID string) {
	plugin.mutex.Lock()
	plugin.scopeLocked(workspaceID).running--
	plugin.signalChangedLocked()
	plugin.mutex.Unlock()
}

func (plugin *Plugin) prepareTaskHint(operationContext context.Context, observed observation, configuration configuration) (operationError error) {
	session := observed.session
	task, operationError := plugin.services.Tasks.ReadTaskState(operationContext, session.InstanceID, session.ID)
	if operationError != nil {
		return operationError
	}
	taskKey := doingFingerprint(task.Doing)
	if taskKey == "" || taskKey != doingFingerprint(observed.state.Doing) {
		return errRetired
	}
	user, operationError := plugin.latestUserMessage(operationContext, session)
	if operationError != nil {
		return operationError
	}
	if user.ID == "" {
		return nil
	}
	// A previous turn's activity is not fresh work for a newly admitted user input.
	if !task.UpdatedAt.IsZero() && !user.CreatedAt.IsZero() && task.UpdatedAt.Before(user.CreatedAt) {
		return nil
	}
	configurationHash := configurationFingerprint(configuration, plugin.promptVersion)
	key := digestText(taskKey + "\n" + user.ID + "\n" + configurationHash)
	previous, operationError := plugin.database.ReadSession(operationContext, session.InstanceID, string(session.ID))
	if operationError != nil {
		return operationError
	}
	if previous.Deleted || previous.Fenced {
		return errRetired
	}
	if previous.LastKey == key && previous.LastStatus != "running" {
		return nil
	}
	if previous.LastKey == key && previous.LastStatus == "running" && time.Now().Before(previous.LastStarted.Add(time.Duration(configuration.JobTimeoutMilliseconds)*time.Millisecond)) {
		return nil
	}
	if !previous.LastStarted.IsZero() {
		delay := time.Until(previous.LastStarted.Add(time.Duration(configuration.CooldownMilliseconds) * time.Millisecond))
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-operationContext.Done():
				timer.Stop()
				return operationContext.Err()
			case <-timer.C:
			}
		}
	}
	run := workerRun{ID: newIdentifier(), WorkspaceID: session.InstanceID, SessionID: session.ID, Key: key, Status: "running", StartedAt: time.Now().UTC()}
	run.ExpiresAt = run.StartedAt.Add(time.Duration(configuration.JobTimeoutMilliseconds) * time.Millisecond)
	if deadline, found := operationContext.Deadline(); found {
		run.ExpiresAt = deadline
	}
	state, operationError := plugin.database.UpdateSession(operationContext, session.InstanceID, string(session.ID), func(state *sessionState) (mutation, error) {
		if state.Deleted || state.Fenced {
			return mutation{}, errRetired
		}
		if state.SourceUser != user.ID || state.TaskKey != taskKey || state.ConfigurationHash != configurationHash {
			state.Epoch++
			state.Pending = nil
		}
		state.SourceUser = user.ID
		state.TaskKey = taskKey
		state.ConfigurationHash = configurationHash
		state.LastKey = key
		state.LastStatus = "running"
		state.LastStarted = run.StartedAt
		run.Epoch = state.Epoch
		return mutation{Run: &run, Counters: map[string]int64{"jobs/started": 1}}, nil
	})
	if operationError != nil {
		return operationError
	}
	defer func() {
		if operationError == nil {
			return
		}
		run.Status = "error"
		if errors.Is(operationError, errRetired) {
			run.Status = "stale"
		}
		if errors.Is(operationError, context.Canceled) || errors.Is(operationError, context.DeadlineExceeded) {
			run.Status = "cancelled"
		}
		run.Error = operationError.Error()
		run.CompletedAt = time.Now().UTC()
		writeContext, cancel := context.WithTimeout(context.WithoutCancel(operationContext), 5*time.Second)
		defer cancel()
		_, writeError := plugin.database.UpdateSession(writeContext, session.InstanceID, string(session.ID), func(current *sessionState) (mutation, error) {
			if current.Deleted || current.Fenced || current.Epoch != run.Epoch || current.LastKey != key {
				return mutation{}, errRetired
			}
			current.LastStatus = run.Status
			return mutation{Run: &run, Counters: map[string]int64{"jobs/" + run.Status: 1}}, nil
		})
		if writeError != nil && !errors.Is(writeError, errRetired) {
			plugin.recordError(session.InstanceID, writeError)
		}
	}()
	sources, counts, operationError := plugin.retrieveSources(operationContext, session, task, user, configuration, state, run.ID)
	if operationError != nil {
		return operationError
	}
	run.MemoryCount = int(counts["sources/memories"])
	run.FileCount = int(counts["sources/files"])
	text, used, operationError := plugin.filterSources(operationContext, session, task, user, configuration, sources, run.ID)
	if operationError != nil {
		return operationError
	}
	if operationError = plugin.validateCurrentSource(operationContext, session, taskKey, user.ID, configurationHash); operationError != nil {
		return operationError
	}
	run.Status = "skipped"
	if text != "" {
		run.Status = "ready"
	}
	run.CompletedAt = time.Now().UTC()
	counts["jobs/"+run.Status]++
	counts["jobs/duration_ms"] += time.Since(run.StartedAt).Milliseconds()
	var pending *preparedHint
	if text != "" {
		pending = &preparedHint{ID: run.ID, Key: key, Epoch: run.Epoch, SourceUser: user.ID, TaskKey: taskKey, ConfigurationHash: configurationHash, Text: text, CreatedAt: run.CompletedAt}
		for _, source := range used {
			pending.Sources = append(pending.Sources, source.Key)
			if source.File != nil {
				pending.Files = append(pending.Files, *source.File)
			}
			if source.Memory != nil {
				pending.Memories = append(pending.Memories, *source.Memory)
			}
		}
		valid, verifyError := plugin.verifyHintSources(operationContext, session, *pending)
		if verifyError != nil {
			return verifyError
		}
		if !valid {
			return errRetired
		}
	}
	_, operationError = plugin.database.UpdateSession(operationContext, session.InstanceID, string(session.ID), func(current *sessionState) (mutation, error) {
		if current.Deleted || current.Fenced || current.Epoch != run.Epoch || current.LastKey != key {
			return mutation{}, errRetired
		}
		current.Pending = pending
		current.LastStatus = run.Status
		return mutation{Run: &run, Counters: counts}, nil
	})
	if operationError == nil && counts["retrieval/memory_errors"] == 0 && counts["retrieval/file_errors"] == 0 {
		plugin.mutex.Lock()
		plugin.scopeLocked(session.InstanceID).lastError = ""
		plugin.mutex.Unlock()
	}
	return operationError
}

func (plugin *Plugin) latestUserMessage(operationContext context.Context, session atom.Session) (atom.Message, error) {
	current, operationError := plugin.services.Conversations.Get(operationContext, session.ID)
	if operationError != nil {
		return atom.Message{}, operationError
	}
	if current.InstanceID != session.InstanceID {
		return atom.Message{}, errRetired
	}
	messages, operationError := plugin.services.Conversations.Messages(operationContext, session.ID)
	if operationError != nil {
		return atom.Message{}, operationError
	}
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == atom.RoleUser && !messages[index].Ephemeral {
			return messages[index], nil
		}
	}
	return atom.Message{}, nil
}

func (plugin *Plugin) validateCurrentSource(operationContext context.Context, session atom.Session, taskKey, sourceUser, configurationHash string) error {
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	task, operationError := plugin.services.Tasks.ReadTaskState(operationContext, session.InstanceID, session.ID)
	if operationError != nil {
		return operationError
	}
	if doingFingerprint(task.Doing) != taskKey {
		return errRetired
	}
	user, operationError := plugin.latestUserMessage(operationContext, session)
	if operationError != nil {
		return operationError
	}
	if user.ID != sourceUser {
		return errRetired
	}
	configuration, operationError := plugin.requestConfiguration(operationContext, session.InstanceID)
	if operationError != nil {
		return operationError
	}
	if !configuration.Enabled || configurationFingerprint(configuration, plugin.promptVersion) != configurationHash {
		return errRetired
	}
	workspace, operationError := plugin.services.Workspaces.Workspace(operationContext, session.InstanceID)
	if operationError != nil {
		return operationError
	}
	if workspace.Stopped {
		return fmt.Errorf("sidekick workspace is stopped")
	}
	return nil
}
