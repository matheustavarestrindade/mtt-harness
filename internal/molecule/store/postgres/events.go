package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type events struct{ store *Store }

func (eventStore *events) LatestSessionEvent(operationContext context.Context, sessionID atom.SessionID, name atom.EventName) (*atom.Event, error) {
	rows, operationError := eventStore.store.pool.Query(operationContext, `SELECT seq,instance_id,session_id,name,payload,created_at FROM events WHERE session_id=$1 AND name=$2 ORDER BY seq DESC LIMIT 1`, string(sessionID), string(name))
	if operationError != nil {
		return nil, operationError
	}
	events, operationError := scanEvents(rows)
	if operationError != nil || len(events) == 0 {
		return nil, operationError
	}
	return &events[0], nil
}

func (eventStore *events) RecentSession(operationContext context.Context, sessionID atom.SessionID, limit int) ([]atom.Event, error) {
	if limit < 1 || limit > 1000 {
		return nil, fmt.Errorf("event limit must be between 1 and 1000")
	}
	rows, operationError := eventStore.store.pool.Query(operationContext, `SELECT seq,instance_id,session_id,name,CASE WHEN octet_length(payload::text)>65536 THEN jsonb_build_object('omitted_bytes',octet_length(payload::text)) ELSE payload END,created_at FROM events WHERE session_id=$1 ORDER BY seq DESC LIMIT $2`, string(sessionID), limit)
	if operationError != nil {
		return nil, operationError
	}
	result, operationError := scanEvents(rows)
	slices.Reverse(result)
	return result, operationError
}

func (eventStore *events) Append(operationContext context.Context, event atom.Event) error {
	_, operationError := eventStore.Record(operationContext, event)
	return operationError
}

func (eventStore *events) Record(operationContext context.Context, event atom.Event) (atom.Event, error) {
	operationError := eventStore.store.pool.QueryRow(operationContext, `
		WITH session_guard AS MATERIALIZED (SELECT deleted FROM sessions WHERE id=$2 FOR SHARE),
		event_gate AS MATERIALIZED (
		 SELECT pg_advisory_xact_lock(hashtextextended('mtt:event-session:' || $2, 0))
		 WHERE NOT EXISTS (SELECT 1 FROM session_guard WHERE deleted)
		)
		INSERT INTO events (instance_id, session_id, name, payload, created_at)
		SELECT $1, $2, $3, $4, $5 FROM event_gate RETURNING seq`,
		event.InstanceID, string(event.SessionID), string(event.Name), event.Payload, event.Time).Scan(&event.Seq)
	if errors.Is(operationError, pgx.ErrNoRows) {
		return event, store.ErrSessionDeleted
	}
	return event, operationError
}

func (eventStore *events) Since(operationContext context.Context, instanceID string, sequenceNumber uint64) ([]atom.Event, error) {
	rows, operationError := eventStore.store.pool.Query(operationContext, `
		SELECT seq, instance_id, session_id, name, payload, created_at
		FROM events WHERE seq > $1 AND ($2 = '' OR instance_id = $2) ORDER BY seq LIMIT 1000`, sequenceNumber, instanceID)
	if operationError != nil {
		return nil, operationError
	}
	return scanEvents(rows)
}

// Session-scoped cursors do not advance past an uncommitted event belonging to
// another session. Record serializes sequence allocation and commit per session.
func (eventStore *events) SinceSession(operationContext context.Context, sessionID atom.SessionID, sequenceNumber uint64) ([]atom.Event, error) {
	rows, operationError := eventStore.store.pool.Query(operationContext, `
	 SELECT seq, instance_id, session_id, name, payload, created_at
	 FROM events WHERE session_id=$1 AND seq>$2 ORDER BY seq LIMIT 1000`, string(sessionID), sequenceNumber)
	if operationError != nil {
		return nil, operationError
	}
	return scanEvents(rows)
}

func scanEvents(rows pgx.Rows) ([]atom.Event, error) {
	defer rows.Close()
	var list []atom.Event
	for rows.Next() {
		var event atom.Event
		if operationError := rows.Scan(&event.Seq, &event.InstanceID, &event.SessionID, &event.Name, &event.Payload, &event.Time); operationError != nil {
			return nil, operationError
		}
		list = append(list, event)
	}
	return list, rows.Err()
}
