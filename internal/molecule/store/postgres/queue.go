package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type queueStore struct{ store *Store }

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
