package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

// Row locks conflict with session writers' FOR SHARE guards. Retained ID/parent
// tombstones prevent late writes and preserve the usage accounting hierarchy.
func (sessionStore *sessions) DeleteConversation(operationContext context.Context, sessionID atom.SessionID) ([]atom.SessionID, error) {
	transaction, operationError := sessionStore.store.pool.Begin(operationContext)
	if operationError != nil {
		return nil, operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	// Match the queue's lock ordering before locking session rows.
	if _, operationError := transaction.Exec(operationContext, `SELECT pg_advisory_xact_lock(672790428)`); operationError != nil {
		return nil, operationError
	}
	rows, operationError := transaction.Query(operationContext, `WITH RECURSIVE tree AS (
		SELECT id,instance_id FROM sessions WHERE id=$1 AND NOT deleted
		UNION SELECT child.id,child.instance_id FROM sessions child JOIN tree parent ON child.parent_id=parent.id AND child.instance_id=parent.instance_id
	) SELECT session.id FROM sessions session JOIN tree ON session.id=tree.id ORDER BY session.id FOR UPDATE OF session`, string(sessionID))
	if operationError != nil {
		return nil, operationError
	}
	identifiers, operationError := pgx.CollectRows(rows, pgx.RowTo[string])
	if operationError != nil {
		return nil, operationError
	}
	if len(identifiers) == 0 {
		return nil, store.ErrSessionNotFound
	}
	var busy bool
	operationError = transaction.QueryRow(operationContext, `SELECT EXISTS(SELECT 1 FROM message_queue WHERE session_id=ANY($1::text[]))
		OR EXISTS(SELECT 1 FROM processes WHERE session_id=ANY($1::text[]) AND status='running')`, identifiers).Scan(&busy)
	if operationError != nil {
		return nil, operationError
	}
	if busy {
		return nil, store.ErrConversationBusy
	}
	for _, statement := range []string{
		`DELETE FROM session_task_state WHERE session_id=ANY($1::text[])`,
		`DELETE FROM messages WHERE session_id=ANY($1::text[])`,
		`DELETE FROM events WHERE session_id=ANY($1::text[])`,
		`DELETE FROM processes WHERE session_id=ANY($1::text[])`,
		`DELETE FROM permissions WHERE session_id=ANY($1::text[]) AND scope<>'always'`,
		`UPDATE permissions SET session_id='' WHERE session_id=ANY($1::text[]) AND scope='always'`,
	} {
		if _, operationError := transaction.Exec(operationContext, statement, identifiers); operationError != nil {
			return nil, operationError
		}
	}
	rows, operationError = transaction.Query(operationContext, `UPDATE sessions SET deleted=true, completed=true, model='', reasoning_effort=''
		WHERE id=ANY($1::text[]) AND NOT deleted RETURNING id`, identifiers)
	if operationError != nil {
		return nil, operationError
	}
	deleted, operationError := pgx.CollectRows(rows, pgx.RowTo[atom.SessionID])
	if operationError != nil {
		return nil, operationError
	}
	if operationError := transaction.Commit(operationContext); operationError != nil {
		return nil, operationError
	}
	return deleted, nil
}
