package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/taskstate"
)

type taskStates struct{ database *Store }

func decodeTaskStateData(state atom.TaskState, todo, doing []byte) (atom.TaskState, error) {
	if operationError := json.Unmarshal(todo, &state.Todo); operationError != nil {
		return atom.TaskState{}, operationError
	}
	if len(doing) > 0 {
		if operationError := json.Unmarshal(doing, &state.Doing); operationError != nil {
			return atom.TaskState{}, operationError
		}
	}
	if state.Todo == nil {
		state.Todo = []atom.TaskItem{}
	}
	return state, nil
}

func (repository *taskStates) Get(operationContext context.Context, sessionID atom.SessionID) (atom.TaskState, error) {
	state := taskstate.Empty(sessionID)
	var todo, doing []byte
	var updatedAt *time.Time
	var deleted bool
	operationError := repository.database.pool.QueryRow(operationContext, `SELECT session.deleted,COALESCE(tasks.todo,'[]'::jsonb),tasks.doing,
		COALESCE(tasks.revision,0),COALESCE(tasks.responses_since_update,0),tasks.updated_at
		FROM sessions session LEFT JOIN session_task_state tasks ON tasks.session_id=session.id WHERE session.id=$1`, string(sessionID)).
		Scan(&deleted, &todo, &doing, &state.Revision, &state.ResponsesSinceUpdate, &updatedAt)
	if errors.Is(operationError, pgx.ErrNoRows) {
		return state, nil
	}
	if operationError != nil {
		return atom.TaskState{}, operationError
	}
	if deleted {
		return atom.TaskState{}, store.ErrSessionDeleted
	}
	if updatedAt != nil {
		state.UpdatedAt = *updatedAt
	}
	return decodeTaskStateData(state, todo, doing)
}

// This lock joins the deletion fence before a task row is created or changed.
func lockTaskSession(operationContext context.Context, transaction pgx.Tx, sessionID atom.SessionID) error {
	var deleted bool
	operationError := transaction.QueryRow(operationContext, `SELECT deleted FROM sessions WHERE id=$1 FOR SHARE`, string(sessionID)).Scan(&deleted)
	if errors.Is(operationError, pgx.ErrNoRows) {
		return store.ErrSessionNotFound
	}
	if operationError != nil {
		return operationError
	}
	if deleted {
		return store.ErrSessionDeleted
	}
	return nil
}

func readLockedTaskState(operationContext context.Context, transaction pgx.Tx, sessionID atom.SessionID) (atom.TaskState, error) {
	state := taskstate.Empty(sessionID)
	var todo, doing []byte
	operationError := transaction.QueryRow(operationContext, `SELECT todo,doing,revision,responses_since_update,updated_at FROM session_task_state WHERE session_id=$1 FOR UPDATE`, string(sessionID)).
		Scan(&todo, &doing, &state.Revision, &state.ResponsesSinceUpdate, &state.UpdatedAt)
	if operationError != nil {
		return atom.TaskState{}, operationError
	}
	return decodeTaskStateData(state, todo, doing)
}

func writeTaskState(operationContext context.Context, transaction pgx.Tx, state atom.TaskState) error {
	todo, operationError := json.Marshal(state.Todo)
	if operationError != nil {
		return operationError
	}
	var doing any
	if state.Doing != nil {
		data, operationError := json.Marshal(state.Doing)
		if operationError != nil {
			return operationError
		}
		doing = data
	}
	_, operationError = transaction.Exec(operationContext, `UPDATE session_task_state SET todo=$2,doing=$3,revision=$4,responses_since_update=$5,updated_at=$6 WHERE session_id=$1`,
		string(state.SessionID), todo, doing, state.Revision, state.ResponsesSinceUpdate, state.UpdatedAt)
	return operationError
}

func (repository *taskStates) Update(operationContext context.Context, sessionID atom.SessionID, update atom.TaskStateUpdate) (atom.TaskState, error) {
	if operationError := taskstate.ValidateUpdate(update); operationError != nil {
		return atom.TaskState{}, operationError
	}
	transaction, operationError := repository.database.pool.Begin(operationContext)
	if operationError != nil {
		return atom.TaskState{}, operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	if operationError := lockTaskSession(operationContext, transaction, sessionID); operationError != nil {
		return atom.TaskState{}, operationError
	}
	if _, operationError := transaction.Exec(operationContext, `INSERT INTO session_task_state(session_id) VALUES($1) ON CONFLICT DO NOTHING`, string(sessionID)); operationError != nil {
		return atom.TaskState{}, operationError
	}
	current, operationError := readLockedTaskState(operationContext, transaction, sessionID)
	if operationError != nil {
		return atom.TaskState{}, operationError
	}
	next, operationError := taskstate.ApplyUpdate(current, update, time.Now())
	if operationError != nil {
		return atom.TaskState{}, operationError
	}
	if operationError := writeTaskState(operationContext, transaction, next); operationError != nil {
		return atom.TaskState{}, operationError
	}
	if operationError := transaction.Commit(operationContext); operationError != nil {
		return atom.TaskState{}, operationError
	}
	return next, nil
}

func (repository *taskStates) RecordResponse(operationContext context.Context, sessionID atom.SessionID, revision int64) error {
	transaction, operationError := repository.database.pool.Begin(operationContext)
	if operationError != nil {
		return operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	if operationError := lockTaskSession(operationContext, transaction, sessionID); operationError != nil {
		return operationError
	}
	_, operationError = transaction.Exec(operationContext, `UPDATE session_task_state SET responses_since_update=LEAST($3,responses_since_update+1)
		WHERE session_id=$1 AND revision=$2 AND (jsonb_array_length(todo)>0 OR doing IS NOT NULL)`, string(sessionID), revision, taskstate.ReminderResponses)
	if operationError != nil {
		return operationError
	}
	return transaction.Commit(operationContext)
}

func (repository *taskStates) Pause(operationContext context.Context, sessionID atom.SessionID) (atom.TaskState, bool, error) {
	transaction, operationError := repository.database.pool.Begin(operationContext)
	if operationError != nil {
		return atom.TaskState{}, false, operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	if operationError := lockTaskSession(operationContext, transaction, sessionID); operationError != nil {
		if errors.Is(operationError, store.ErrSessionNotFound) {
			return taskstate.Empty(sessionID), false, nil
		}
		return atom.TaskState{}, false, operationError
	}
	current, operationError := readLockedTaskState(operationContext, transaction, sessionID)
	if errors.Is(operationError, pgx.ErrNoRows) {
		return taskstate.Empty(sessionID), false, nil
	}
	if operationError != nil {
		return atom.TaskState{}, false, operationError
	}
	if current.Doing == nil {
		return current, false, nil
	}
	next, operationError := taskstate.ApplyUpdate(current, atom.TaskStateUpdate{DoingProvided: true}, time.Now())
	if operationError != nil {
		return atom.TaskState{}, false, operationError
	}
	if operationError := writeTaskState(operationContext, transaction, next); operationError != nil {
		return atom.TaskState{}, false, operationError
	}
	if operationError := transaction.Commit(operationContext); operationError != nil {
		return atom.TaskState{}, false, operationError
	}
	return next, true, nil
}
