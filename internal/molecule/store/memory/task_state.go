package memory

import (
	"context"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/taskstate"
)

type taskStates struct{ database *Store }

func (repository *taskStates) Get(operationContext context.Context, sessionID atom.SessionID) (atom.TaskState, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return atom.TaskState{}, operationError
	}
	repository.database.mutex.RLock()
	defer repository.database.mutex.RUnlock()
	if operationError := operationContext.Err(); operationError != nil {
		return atom.TaskState{}, operationError
	}
	if repository.database.deletedSessions[sessionID] {
		return atom.TaskState{}, store.ErrSessionDeleted
	}
	state, found := repository.database.taskStates[sessionID]
	if !found {
		return taskstate.Empty(sessionID), nil
	}
	return taskstate.Clone(state), nil
}

func (repository *taskStates) Update(operationContext context.Context, sessionID atom.SessionID, update atom.TaskStateUpdate) (atom.TaskState, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return atom.TaskState{}, operationError
	}
	repository.database.mutex.Lock()
	defer repository.database.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return atom.TaskState{}, operationError
	}
	if repository.database.deletedSessions[sessionID] {
		return atom.TaskState{}, store.ErrSessionDeleted
	}
	if _, found := repository.database.sessions[sessionID]; !found {
		return atom.TaskState{}, store.ErrSessionNotFound
	}
	current, found := repository.database.taskStates[sessionID]
	if !found {
		current = taskstate.Empty(sessionID)
	}
	next, operationError := taskstate.ApplyUpdate(current, update, time.Now())
	if operationError != nil {
		return atom.TaskState{}, operationError
	}
	repository.database.taskStates[sessionID] = next
	return taskstate.Clone(next), nil
}

func (repository *taskStates) RecordResponse(operationContext context.Context, sessionID atom.SessionID, revision int64) error {
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	repository.database.mutex.Lock()
	defer repository.database.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	if repository.database.deletedSessions[sessionID] {
		return store.ErrSessionDeleted
	}
	state, found := repository.database.taskStates[sessionID]
	if !found || state.Revision != revision || !taskstate.Active(state) {
		return nil
	}
	state.ResponsesSinceUpdate = min(taskstate.ReminderResponses, state.ResponsesSinceUpdate+1)
	repository.database.taskStates[sessionID] = state
	return nil
}

func (repository *taskStates) Pause(operationContext context.Context, sessionID atom.SessionID) (atom.TaskState, bool, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return atom.TaskState{}, false, operationError
	}
	repository.database.mutex.Lock()
	defer repository.database.mutex.Unlock()
	if operationError := operationContext.Err(); operationError != nil {
		return atom.TaskState{}, false, operationError
	}
	if repository.database.deletedSessions[sessionID] {
		return atom.TaskState{}, false, store.ErrSessionDeleted
	}
	state, found := repository.database.taskStates[sessionID]
	if !found {
		return taskstate.Empty(sessionID), false, nil
	}
	if state.Doing == nil {
		return taskstate.Clone(state), false, nil
	}
	next, operationError := taskstate.ApplyUpdate(state, atom.TaskStateUpdate{DoingProvided: true}, time.Now())
	if operationError != nil {
		return atom.TaskState{}, false, operationError
	}
	repository.database.taskStates[sessionID] = next
	return taskstate.Clone(next), true, nil
}
