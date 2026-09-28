package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type permissions struct{ store *Store }

func (permissionStore *permissions) Save(operationContext context.Context, decision atom.PermissionDecision) error {
	_, operationError := permissionStore.store.pool.Exec(operationContext, `
		INSERT INTO permissions (request_id, kind, scope, created_at, instance_id, session_id, target)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (request_id) DO UPDATE SET kind = EXCLUDED.kind, scope = EXCLUDED.scope`,
		decision.RequestID, string(decision.Kind), string(decision.Scope), decision.CreatedAt, decision.InstanceID, string(decision.SessionID), decision.Target)
	return operationError
}

func (permissionStore *permissions) Get(operationContext context.Context, identifier string) (atom.PermissionDecision, error) {
	var decision atom.PermissionDecision
	operationError := permissionStore.store.pool.QueryRow(operationContext, `
		SELECT request_id, kind, scope, created_at, instance_id, session_id, target FROM permissions WHERE request_id = $1`, identifier).
		Scan(&decision.RequestID, &decision.Kind, &decision.Scope, &decision.CreatedAt, &decision.InstanceID, &decision.SessionID, &decision.Target)
	return decision, operationError
}

func (permissionStore *permissions) Resolve(operationContext context.Context, session atom.Session, target string) (atom.PermissionDecision, bool, error) {
	var decision atom.PermissionDecision
	operationError := permissionStore.store.pool.QueryRow(operationContext, `
		SELECT request_id, kind, scope, created_at, instance_id, session_id, target
		FROM permissions WHERE instance_id=$1 AND target=$2
		AND (scope='always' OR (scope='session' AND session_id=$3))
		ORDER BY (scope='session') DESC, created_at DESC, request_id DESC LIMIT 1`,
		session.InstanceID, target, string(session.ID)).Scan(&decision.RequestID, &decision.Kind, &decision.Scope, &decision.CreatedAt, &decision.InstanceID, &decision.SessionID, &decision.Target)
	if errors.Is(operationError, pgx.ErrNoRows) {
		return decision, false, nil
	}
	return decision, operationError == nil, operationError
}
