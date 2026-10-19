package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type instances struct{ store *Store }

func (instanceStore *instances) Save(operationContext context.Context, instanceSpec atom.InstanceSpec) error {
	models, _ := json.Marshal(instanceSpec.Models)
	result, operationError := instanceStore.store.pool.Exec(operationContext, `
		INSERT INTO instances (id, workspace, models, default_model, process_limit, agent_depth_limit, created_at, stopped)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			workspace = EXCLUDED.workspace,
			models = EXCLUDED.models,
			default_model = EXCLUDED.default_model,
			process_limit = EXCLUDED.process_limit,
			agent_depth_limit = EXCLUDED.agent_depth_limit,
			stopped = EXCLUDED.stopped
		WHERE NOT instances.unlinked`,
		instanceSpec.ID, instanceSpec.Workspace, models, instanceSpec.DefaultModel, instanceSpec.ProcessLimit, instanceSpec.AgentDepthLimit, instanceSpec.CreatedAt, instanceSpec.Stopped)
	if operationError != nil {
		return operationError
	}
	if result.RowsAffected() == 0 {
		return store.ErrInstanceUnlinked
	}
	return nil
}

func (instanceStore *instances) Get(operationContext context.Context, identifier string) (atom.InstanceSpec, error) {
	var instanceSpec atom.InstanceSpec
	var models []byte
	operationError := instanceStore.store.pool.QueryRow(operationContext, `
		SELECT id, workspace, models, default_model, process_limit, agent_depth_limit, created_at, stopped
		FROM instances WHERE id = $1 AND NOT unlinked`, identifier).
		Scan(&instanceSpec.ID, &instanceSpec.Workspace, &models, &instanceSpec.DefaultModel, &instanceSpec.ProcessLimit, &instanceSpec.AgentDepthLimit, &instanceSpec.CreatedAt, &instanceSpec.Stopped)
	if operationError != nil {
		if errors.Is(operationError, pgx.ErrNoRows) {
			return atom.InstanceSpec{}, store.ErrInstanceNotFound
		}
		return atom.InstanceSpec{}, operationError
	}
	_ = json.Unmarshal(models, &instanceSpec.Models)
	return instanceSpec, nil
}

func (instanceStore *instances) All(operationContext context.Context) ([]atom.InstanceSpec, error) {
	rows, operationError := instanceStore.store.pool.Query(operationContext, `
		SELECT id, workspace, models, default_model, process_limit, agent_depth_limit, created_at, stopped
		FROM instances WHERE NOT unlinked ORDER BY id`)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var list []atom.InstanceSpec
	for rows.Next() {
		var instanceSpec atom.InstanceSpec
		var models []byte
		if operationError := rows.Scan(&instanceSpec.ID, &instanceSpec.Workspace, &models, &instanceSpec.DefaultModel, &instanceSpec.ProcessLimit, &instanceSpec.AgentDepthLimit, &instanceSpec.CreatedAt, &instanceSpec.Stopped); operationError != nil {
			return nil, operationError
		}
		_ = json.Unmarshal(models, &instanceSpec.Models)
		list = append(list, instanceSpec)
	}
	return list, rows.Err()
}

func (instanceStore *instances) Delete(operationContext context.Context, identifier string) error {
	_, operationError := instanceStore.store.pool.Exec(operationContext, `DELETE FROM instances WHERE id = $1 AND NOT unlinked`, identifier)
	return operationError
}

func (instanceStore *instances) Unlink(operationContext context.Context, identifier string) error {
	transaction, operationError := instanceStore.store.pool.Begin(operationContext)
	if operationError != nil {
		return operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	var unlinked bool
	operationError = transaction.QueryRow(operationContext, `SELECT unlinked FROM instances WHERE id=$1 FOR UPDATE`, identifier).Scan(&unlinked)
	if errors.Is(operationError, pgx.ErrNoRows) || operationError == nil && unlinked {
		return store.ErrInstanceNotFound
	}
	if operationError != nil {
		return operationError
	}
	var busy bool
	operationError = transaction.QueryRow(operationContext, `SELECT EXISTS(SELECT 1 FROM message_queue pending JOIN sessions session ON session.id=pending.session_id WHERE session.instance_id=$1) OR EXISTS(SELECT 1 FROM processes WHERE instance_id=$1 AND status='running')`, identifier).Scan(&busy)
	if operationError != nil {
		return operationError
	}
	if busy {
		return store.ErrInstanceBusy
	}
	if _, operationError = transaction.Exec(operationContext, `UPDATE instances SET stopped=true,unlinked=true WHERE id=$1`, identifier); operationError != nil {
		return operationError
	}
	return transaction.Commit(operationContext)
}
