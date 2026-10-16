package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type queueStore struct{ store *Store }

func (queue *queueStore) PendingMessage(operationContext context.Context, sessionID atom.SessionID, messageID string) (atom.Message, error) {
	var data []byte
	operationError := queue.store.pool.QueryRow(operationContext, `SELECT queued.message FROM message_queue queued LEFT JOIN sessions session ON session.id=queued.session_id WHERE queued.id=$1 AND queued.session_id=$2 AND NOT queued.running AND NOT COALESCE(session.deleted,false)`, messageID, string(sessionID)).Scan(&data)
	if errors.Is(operationError, pgx.ErrNoRows) {
		return atom.Message{}, store.ErrQueuedMessageNotFound
	}
	if operationError != nil {
		return atom.Message{}, operationError
	}
	var message atom.Message
	operationError = json.Unmarshal(data, &message)
	return message, operationError
}

func (queue *queueStore) StartedBatch(operationContext context.Context, sessionID atom.SessionID, messageIDs []string) ([]string, error) {
	if operationError := store.ValidateQueuedBatch(messageIDs); operationError != nil {
		return nil, operationError
	}
	rows, operationError := queue.store.pool.Query(operationContext, `SELECT id FROM message_queue WHERE session_id=$1 AND id=ANY($2::text[]) AND running ORDER BY seq`, string(sessionID), messageIDs)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var started []string
	for rows.Next() {
		var identifier string
		if operationError = rows.Scan(&identifier); operationError != nil {
			return nil, operationError
		}
		started = append(started, identifier)
	}
	return started, rows.Err()
}

func (queue *queueStore) Enqueue(operationContext context.Context, message atom.Message, limit int) error {
	data, operationError := json.Marshal(message)
	if operationError != nil {
		return operationError
	}
	transaction, operationError := queue.store.pool.Begin(operationContext)
	if operationError != nil {
		return operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	if _, operationError := transaction.Exec(operationContext, `SELECT pg_advisory_xact_lock(672790428)`); operationError != nil {
		return operationError
	}
	var deleted bool
	if operationError := transaction.QueryRow(operationContext, `WITH session_guard AS MATERIALIZED (
		SELECT deleted FROM sessions WHERE id=$1 FOR SHARE
	) SELECT EXISTS(SELECT 1 FROM session_guard WHERE deleted)`, string(message.SessionID)).Scan(&deleted); operationError != nil {
		return operationError
	}
	if deleted {
		return store.ErrSessionDeleted
	}
	var count, total int
	if operationError := transaction.QueryRow(operationContext, `SELECT COUNT(*) FILTER (WHERE session_id=$1 AND NOT running), COUNT(*) FROM message_queue`, string(message.SessionID)).Scan(&count, &total); operationError != nil {
		return operationError
	}
	if count >= limit || total >= 4096 {
		return store.ErrQueueFull
	}
	if _, operationError := transaction.Exec(operationContext, `INSERT INTO message_queue(id,session_id,message) VALUES($1,$2,$3)`, message.ID, string(message.SessionID), data); operationError != nil {
		return operationError
	}
	return transaction.Commit(operationContext)
}

func (queue *queueStore) All(operationContext context.Context) ([]atom.QueuedMessage, error) {
	rows, operationError := queue.store.pool.Query(operationContext, `SELECT message,running,seq FROM message_queue ORDER BY seq`)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var entries []atom.QueuedMessage
	for rows.Next() {
		var entry atom.QueuedMessage
		var data []byte
		if operationError := rows.Scan(&data, &entry.Running, &entry.Sequence); operationError != nil {
			return nil, operationError
		}
		if operationError := json.Unmarshal(data, &entry.Message); operationError != nil {
			return nil, operationError
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// Claim and append are one transaction: a crash cannot acknowledge a run start
// without its user message, nor append that message twice during recovery.
func (queue *queueStore) Start(operationContext context.Context, messageID string) error {
	transaction, operationError := queue.store.pool.Begin(operationContext)
	if operationError != nil {
		return operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	var data []byte
	if operationError := transaction.QueryRow(operationContext, `UPDATE message_queue SET running=true WHERE id=$1 AND NOT running RETURNING message`, messageID).Scan(&data); operationError != nil {
		return operationError
	}
	var message atom.Message
	if operationError := json.Unmarshal(data, &message); operationError != nil {
		return operationError
	}
	content, operationError := json.Marshal(message.Content)
	if operationError != nil {
		return operationError
	}
	_, operationError = transaction.Exec(operationContext, `INSERT INTO messages(id,session_id,role,content,created_at) VALUES($1,$2,$3,$4,$5)`, message.ID, string(message.SessionID), string(message.Role), content, message.CreatedAt)
	if operationError != nil {
		return operationError
	}
	return transaction.Commit(operationContext)
}

func (queue *queueStore) Finish(operationContext context.Context, messageID string) error {
	_, operationError := queue.store.pool.Exec(operationContext, `DELETE FROM message_queue WHERE id=$1`, messageID)
	return operationError
}

func (queue *queueStore) StartBatch(operationContext context.Context, sessionID atom.SessionID, messageIDs []string) error {
	if operationError := store.ValidateQueuedBatch(messageIDs); operationError != nil {
		return operationError
	}
	transaction, operationError := queue.store.pool.Begin(operationContext)
	if operationError != nil {
		return operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	var deleted bool
	if operationError = transaction.QueryRow(operationContext, `WITH session_guard AS MATERIALIZED (SELECT deleted FROM sessions WHERE id=$1 FOR SHARE) SELECT EXISTS(SELECT 1 FROM session_guard WHERE deleted)`, string(sessionID)).Scan(&deleted); operationError != nil {
		return operationError
	}
	if deleted {
		return store.ErrSessionDeleted
	}
	var previous int64
	for _, identifier := range messageIDs {
		var data []byte
		var sequence int64
		if operationError = transaction.QueryRow(operationContext, `UPDATE message_queue SET running=true WHERE id=$1 AND session_id=$2 AND NOT running RETURNING message,seq`, identifier, string(sessionID)).Scan(&data, &sequence); operationError != nil {
			return operationError
		}
		if sequence <= previous {
			return fmt.Errorf("queued batch is not ordered")
		}
		previous = sequence
		var message atom.Message
		if operationError = json.Unmarshal(data, &message); operationError != nil {
			return operationError
		}
		content, encodeError := json.Marshal(message.Content)
		if encodeError != nil {
			return encodeError
		}
		if _, operationError = transaction.Exec(operationContext, `INSERT INTO messages(id,session_id,role,content,created_at) VALUES($1,$2,$3,$4,$5)`, message.ID, string(sessionID), string(message.Role), content, message.CreatedAt); operationError != nil {
			return operationError
		}
	}
	return transaction.Commit(operationContext)
}
func (queue *queueStore) Remove(operationContext context.Context, sessionID atom.SessionID, messageID string) (bool, error) {
	tag, operationError := queue.store.pool.Exec(operationContext, `DELETE FROM message_queue WHERE id=$1 AND session_id=$2 AND NOT running`, messageID, string(sessionID))
	return tag.RowsAffected() > 0, operationError
}
func (queue *queueStore) ClearPending(operationContext context.Context, sessionID atom.SessionID) (int, error) {
	tag, operationError := queue.store.pool.Exec(operationContext, `DELETE FROM message_queue WHERE session_id=$1 AND NOT running`, string(sessionID))
	if operationError != nil {
		return 0, fmt.Errorf("clear pending messages: %w", operationError)
	}
	return int(tag.RowsAffected()), nil
}
