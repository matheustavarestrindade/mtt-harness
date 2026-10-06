package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type sessions struct{ store *Store }

func (sessionStore *sessions) GetModelSelection(operationContext context.Context, sessionID atom.SessionID) (atom.SessionModelSelection, bool, error) {
	var selection atom.SessionModelSelection
	var deleted bool
	operationError := sessionStore.store.pool.QueryRow(operationContext, `SELECT model,reasoning_effort,deleted FROM sessions WHERE id=$1`, string(sessionID)).Scan(&selection.Model, &selection.ReasoningEffort, &deleted)
	if deleted {
		return atom.SessionModelSelection{}, false, store.ErrSessionDeleted
	}
	if errors.Is(operationError, pgx.ErrNoRows) {
		return atom.SessionModelSelection{}, false, nil
	}
	return selection, operationError == nil, operationError
}

func (sessionStore *sessions) List(operationContext context.Context, instanceID string) ([]atom.Session, error) {
	rows, operationError := sessionStore.store.pool.Query(operationContext, `SELECT id,instance_id,parent_id,depth,model,created_at,completed,reasoning_effort FROM sessions WHERE instance_id=$1 AND NOT deleted ORDER BY id`, instanceID)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var sessions []atom.Session
	for rows.Next() {
		var session atom.Session
		if operationError := rows.Scan(&session.ID, &session.InstanceID, &session.Parent, &session.Depth, &session.Model, &session.CreatedAt, &session.Completed, &session.ReasoningEffort); operationError != nil {
			return nil, operationError
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (sessionStore *sessions) Save(operationContext context.Context, session atom.Session) error {
	result, operationError := sessionStore.store.pool.Exec(operationContext, `
		WITH parent_state AS MATERIALIZED (SELECT deleted FROM sessions WHERE id=$3 FOR SHARE)
		INSERT INTO sessions (id, instance_id, parent_id, depth, model, created_at, completed, reasoning_effort)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8 WHERE NOT EXISTS (SELECT 1 FROM parent_state WHERE deleted)
		ON CONFLICT (id) DO UPDATE SET
			parent_id = EXCLUDED.parent_id,
			depth = EXCLUDED.depth,
			completed = EXCLUDED.completed WHERE NOT sessions.deleted`,
		string(session.ID), session.InstanceID, string(session.Parent), session.Depth, session.Model, session.CreatedAt, session.Completed, session.ReasoningEffort)
	if operationError == nil && result.RowsAffected() == 0 {
		return store.ErrSessionDeleted
	}
	return operationError
}

func (sessionStore *sessions) Get(operationContext context.Context, sessionID atom.SessionID) (atom.Session, error) {
	var session atom.Session
	operationError := sessionStore.store.pool.QueryRow(operationContext, `
		SELECT id, instance_id, parent_id, depth, model, created_at, completed, reasoning_effort
		FROM sessions WHERE id = $1 AND NOT deleted`, string(sessionID)).
		Scan(&session.ID, &session.InstanceID, &session.Parent, &session.Depth, &session.Model, &session.CreatedAt, &session.Completed, &session.ReasoningEffort)
	if errors.Is(operationError, pgx.ErrNoRows) {
		return atom.Session{}, store.ErrSessionNotFound
	}
	return session, operationError
}

func (sessionStore *sessions) SetModelSelection(operationContext context.Context, sessionID atom.SessionID, previous, next atom.SessionModelSelection) error {
	result, operationError := sessionStore.store.pool.Exec(operationContext, `UPDATE sessions SET model=$2, reasoning_effort=$3
		WHERE id=$1 AND model=$4 AND reasoning_effort=$5 AND NOT completed AND NOT deleted`, string(sessionID), next.Model, next.ReasoningEffort, previous.Model, previous.ReasoningEffort)
	if operationError != nil {
		return operationError
	}
	if result.RowsAffected() == 0 {
		return store.ErrSessionSelectionChanged
	}
	return nil
}

func (sessionStore *sessions) Agents(operationContext context.Context, parent atom.SessionID) ([]atom.SessionID, error) {
	query := `SELECT id FROM sessions WHERE parent_id = '' AND NOT deleted ORDER BY id`
	var arguments []any
	if parent != "" {
		query = `SELECT id FROM sessions WHERE parent_id = $1 AND NOT deleted ORDER BY id`
		arguments = []any{string(parent)}
	}
	rows, operationError := sessionStore.store.pool.Query(operationContext, query, arguments...)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var list []atom.SessionID
	for rows.Next() {
		var sessionID atom.SessionID
		if operationError := rows.Scan(&sessionID); operationError != nil {
			return nil, operationError
		}
		list = append(list, sessionID)
	}
	return list, rows.Err()
}

func (sessionStore *sessions) Append(operationContext context.Context, message atom.Message) error {
	content, _ := json.Marshal(message.Content)
	calls, _ := json.Marshal(message.ToolCalls)
	var usage any
	var providerState any
	if message.ProviderState != nil {
		data, operationError := json.Marshal(message.ProviderState)
		if operationError != nil {
			return operationError
		}
		providerState = data
	}
	if message.Usage != nil {
		data, _ := json.Marshal(message.Usage)
		usage = data
	}
	result, operationError := sessionStore.store.pool.Exec(operationContext, `
		WITH session_guard AS MATERIALIZED (SELECT deleted FROM sessions WHERE id=$2 FOR SHARE)
		INSERT INTO messages (id, session_id, role, content, tool_calls, tool_call_id, usage, created_at, provider_state, reasoning)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10 WHERE NOT EXISTS (SELECT 1 FROM session_guard WHERE deleted)`,
		message.ID, string(message.SessionID), string(message.Role), content, calls, message.ToolCallID, usage, message.CreatedAt, providerState, message.Reasoning)
	if operationError == nil && result.RowsAffected() == 0 {
		return store.ErrSessionDeleted
	}
	return operationError
}

func (sessionStore *sessions) DeleteAfter(operationContext context.Context, sessionID atom.SessionID, messageID string) (int, error) {
	transaction, operationError := sessionStore.store.pool.Begin(operationContext)
	if operationError != nil {
		return 0, operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	if operationError := lockTaskSession(operationContext, transaction, sessionID); operationError != nil {
		return 0, operationError
	}
	var sequence int64
	if operationError := transaction.QueryRow(operationContext, `SELECT seq FROM messages WHERE id=$1 AND session_id=$2`, messageID, string(sessionID)).Scan(&sequence); operationError != nil {
		return 0, operationError
	}
	tag, operationError := transaction.Exec(operationContext, `DELETE FROM messages WHERE session_id=$1 AND seq>$2`, string(sessionID), sequence)
	if operationError != nil {
		return 0, operationError
	}
	if _, operationError := transaction.Exec(operationContext, `UPDATE session_task_state SET todo='[]',doing=NULL,revision=revision+1,responses_since_update=0,updated_at=now() WHERE session_id=$1`, string(sessionID)); operationError != nil {
		return 0, operationError
	}
	if operationError := transaction.Commit(operationContext); operationError != nil {
		return 0, operationError
	}
	return int(tag.RowsAffected()), nil
}

func (sessionStore *sessions) Messages(operationContext context.Context, sessionID atom.SessionID) ([]atom.Message, error) {
	rows, operationError := sessionStore.store.pool.Query(operationContext, `
		SELECT id, session_id, role, content, tool_calls, tool_call_id, usage, created_at, provider_state, reasoning
		FROM messages WHERE session_id = $1 ORDER BY seq`, string(sessionID))
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var list []atom.Message
	for rows.Next() {
		var message atom.Message
		var content, calls, usage, providerState []byte
		if operationError := rows.Scan(&message.ID, &message.SessionID, &message.Role, &content, &calls, &message.ToolCallID, &usage, &message.CreatedAt, &providerState, &message.Reasoning); operationError != nil {
			return nil, operationError
		}
		_ = json.Unmarshal(content, &message.Content)
		_ = json.Unmarshal(calls, &message.ToolCalls)
		if len(usage) > 0 {
			_ = json.Unmarshal(usage, &message.Usage)
		}
		if len(providerState) > 0 {
			if operationError := json.Unmarshal(providerState, &message.ProviderState); operationError != nil {
				return nil, operationError
			}
		}
		list = append(list, message)
	}
	return list, rows.Err()
}
