package postgres

import (
	"context"
	"encoding/json"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type sessions struct{ store *Store }

func (sessionStore *sessions) List(operationContext context.Context, instanceID string) ([]atom.Session, error) {
	rows, operationError := sessionStore.store.pool.Query(operationContext, `SELECT id,instance_id,parent_id,depth,model,created_at,completed FROM sessions WHERE instance_id=$1 ORDER BY id`, instanceID)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var sessions []atom.Session
	for rows.Next() {
		var session atom.Session
		if operationError := rows.Scan(&session.ID, &session.InstanceID, &session.Parent, &session.Depth, &session.Model, &session.CreatedAt, &session.Completed); operationError != nil {
			return nil, operationError
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (sessionStore *sessions) Save(operationContext context.Context, session atom.Session) error {
	_, operationError := sessionStore.store.pool.Exec(operationContext, `
		INSERT INTO sessions (id, instance_id, parent_id, depth, model, created_at, completed)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			parent_id = EXCLUDED.parent_id,
			depth = EXCLUDED.depth,
			model = EXCLUDED.model,
			completed = EXCLUDED.completed`,
		string(session.ID), session.InstanceID, string(session.Parent), session.Depth, session.Model, session.CreatedAt, session.Completed)
	return operationError
}

func (sessionStore *sessions) Get(operationContext context.Context, sessionID atom.SessionID) (atom.Session, error) {
	var session atom.Session
	operationError := sessionStore.store.pool.QueryRow(operationContext, `
		SELECT id, instance_id, parent_id, depth, model, created_at, completed
		FROM sessions WHERE id = $1`, string(sessionID)).
		Scan(&session.ID, &session.InstanceID, &session.Parent, &session.Depth, &session.Model, &session.CreatedAt, &session.Completed)
	return session, operationError
}

func (sessionStore *sessions) Agents(operationContext context.Context, parent atom.SessionID) ([]atom.SessionID, error) {
	query := `SELECT id FROM sessions WHERE parent_id = '' ORDER BY id`
	var arguments []any
	if parent != "" {
		query = `SELECT id FROM sessions WHERE parent_id = $1 ORDER BY id`
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
	if message.Usage != nil {
		data, _ := json.Marshal(message.Usage)
		usage = data
	}
	_, operationError := sessionStore.store.pool.Exec(operationContext, `
		INSERT INTO messages (id, session_id, role, content, tool_calls, tool_call_id, usage, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		message.ID, string(message.SessionID), string(message.Role), content, calls, message.ToolCallID, usage, message.CreatedAt)
	return operationError
}

func (sessionStore *sessions) DeleteAfter(operationContext context.Context, sessionID atom.SessionID, messageID string) (int, error) {
	tag, operationError := sessionStore.store.pool.Exec(operationContext, `
		DELETE FROM messages WHERE session_id = $1 AND seq > (SELECT seq FROM messages WHERE id = $2 AND session_id = $1)`,
		string(sessionID), messageID)
	if operationError != nil {
		return 0, operationError
	}
	return int(tag.RowsAffected()), nil
}

func (sessionStore *sessions) Messages(operationContext context.Context, sessionID atom.SessionID) ([]atom.Message, error) {
	rows, operationError := sessionStore.store.pool.Query(operationContext, `
		SELECT id, session_id, role, content, tool_calls, tool_call_id, usage, created_at
		FROM messages WHERE session_id = $1 ORDER BY seq`, string(sessionID))
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var list []atom.Message
	for rows.Next() {
		var message atom.Message
		var content, calls, usage []byte
		if operationError := rows.Scan(&message.ID, &message.SessionID, &message.Role, &content, &calls, &message.ToolCallID, &usage, &message.CreatedAt); operationError != nil {
			return nil, operationError
		}
		_ = json.Unmarshal(content, &message.Content)
		_ = json.Unmarshal(calls, &message.ToolCalls)
		if len(usage) > 0 {
			_ = json.Unmarshal(usage, &message.Usage)
		}
		list = append(list, message)
	}
	return list, rows.Err()
}
