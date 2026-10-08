package spacedrepetition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func configurationFingerprint(value configuration, promptVersion string) string {
	data, _ := json.Marshal(struct {
		Configuration configuration
		Prompts       string
	}{value, promptVersion})
	fingerprint := sha256.Sum256(data)
	return hex.EncodeToString(fingerprint[:])
}

func (plugin *Plugin) recoverInstructions(operationContext context.Context, session atom.Session, callID, reason string) (operationError error) {
	if callID == "" {
		return fmt.Errorf("instruction recovery call ID is empty")
	}
	if operationError := plugin.validateSession(operationContext, session); operationError != nil {
		return operationError
	}
	configuration, operationError := plugin.requestConfiguration(operationContext, session.InstanceID)
	if operationError != nil {
		return operationError
	}
	if !configuration.Enabled || configuration.WorkerModel == "" || plugin.database == nil {
		return fmt.Errorf("instruction recovery is not configured")
	}
	history, operationError := plugin.services.Conversations.Messages(operationContext, session.ID)
	if operationError != nil {
		return operationError
	}
	_, _, sourceUser := requestAnchors(atom.Request{Messages: history})
	state, operationError := plugin.database.UpdateSession(operationContext, session.InstanceID, string(session.ID), func(state *sessionState) (map[string]int64, error) {
		if state.Deleted || state.Fenced {
			return nil, errSessionRetired
		}
		state.SourceUser = sourceUser
		return nil, nil
	})
	if operationError != nil {
		return operationError
	}
	fingerprint := sha256.Sum256([]byte(string(session.ID) + "\x00" + callID))
	identifier := hex.EncodeToString(fingerprint[:])
	previous, operationError := plugin.database.ReadRecovery(operationContext, session.InstanceID, identifier)
	if operationError != nil {
		return operationError
	}
	if previous.ID != "" {
		if previous.Reason != reason || previous.Epoch != state.Epoch || previous.SourceUser != sourceUser {
			return fmt.Errorf("instruction recovery call ID belongs to different input or conversation state")
		}
		if previous.Status == "completed" {
			return nil
		}
		return fmt.Errorf("instruction recovery call has status %s: %s", previous.Status, previous.Error)
	}
	workerContext, cancel := context.WithTimeout(operationContext, time.Duration(configuration.JobTimeoutMilliseconds)*time.Millisecond)
	stopShutdown := context.AfterFunc(plugin.operationContext, cancel)
	plugin.mutex.Lock()
	if plugin.closed {
		plugin.mutex.Unlock()
		stopShutdown()
		cancel()
		return fmt.Errorf("spaced repetition is closed")
	}
	scope := plugin.scopeLocked(session.InstanceID)
	if _, active := scope.jobs[string(session.ID)]; active {
		plugin.mutex.Unlock()
		stopShutdown()
		cancel()
		return fmt.Errorf("instruction recovery is already running for this session")
	}
	if len(scope.jobs) >= configuration.WorkerCount {
		plugin.mutex.Unlock()
		stopShutdown()
		cancel()
		return fmt.Errorf("workspace instruction recovery concurrency limit reached")
	}
	scope.jobs[string(session.ID)] = cancel
	plugin.workers.Add(1)
	plugin.mutex.Unlock()
	defer func() {
		stopShutdown()
		cancel()
		plugin.mutex.Lock()
		delete(scope.jobs, string(session.ID))
		if operationError != nil && !errors.Is(operationError, context.Canceled) {
			scope.lastError = operationError.Error()
		} else if operationError == nil {
			scope.lastError = ""
		}
		plugin.mutex.Unlock()
		plugin.workers.Done()
	}()
	started := time.Now().UTC()
	run := recoveryRun{ID: identifier, WorkspaceID: session.InstanceID, SessionID: string(session.ID), CallID: callID, Reason: reason, Epoch: state.Epoch, SourceUser: sourceUser, Status: "running", Model: configuration.WorkerModel, CreatedAt: started, ExpiresAt: started.Add(time.Duration(configuration.JobTimeoutMilliseconds) * time.Millisecond)}
	if operationError := plugin.database.SaveRecovery(workerContext, run); operationError != nil {
		return operationError
	}
	defer func() {
		if operationError == nil {
			return
		}
		run.Status = "failed"
		if errors.Is(operationError, context.Canceled) || errors.Is(operationError, context.DeadlineExceeded) {
			run.Status = "cancelled"
		}
		run.Error = operationError.Error()
		run.CompletedAt = time.Now().UTC()
		recordContext, cancelRecord := context.WithTimeout(context.WithoutCancel(operationContext), 10*time.Second)
		defer cancelRecord()
		if saveError := plugin.database.SaveRecovery(recordContext, run); saveError != nil && !errors.Is(saveError, errSessionRetired) {
			operationError = errors.Join(operationError, saveError)
		}
	}()
	report, operationError := plugin.prepareRecovery(workerContext, session, configuration, reason, history, &run)
	if operationError != nil {
		return operationError
	}
	if operationError := workerContext.Err(); operationError != nil {
		return operationError
	}
	if operationError := plugin.validateSession(workerContext, session); operationError != nil {
		return operationError
	}
	current, operationError := plugin.services.Conversations.Messages(workerContext, session.ID)
	if operationError != nil {
		return operationError
	}
	_, _, currentUser := requestAnchors(atom.Request{Messages: current})
	if currentUser != sourceUser {
		return fmt.Errorf("user instructions changed during instruction recovery")
	}
	run.Status = "completed"
	run.CompletedAt = time.Now().UTC()
	prepared := recoveryReport{ID: run.ID, Epoch: run.Epoch, SourceUser: sourceUser, Text: report, MemoryAvailable: run.MemoryAvailable, ConfigurationHash: configurationFingerprint(configuration, plugin.promptVersion), CreatedAt: time.Now().UTC()}
	return plugin.database.CompleteRecovery(workerContext, run, prepared, map[string]int64{"recovery/prepared": 1, "recovery/queries": int64(run.Queries), "recovery/memories": int64(run.Memories), "recovery/duration_ms": time.Since(started).Milliseconds()})
}
