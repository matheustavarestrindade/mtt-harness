package postgres

import (
	"context"
	"encoding/json"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type instances struct{ store *Store }

func (instanceStore *instances) Save(operationContext context.Context, instanceSpec atom.InstanceSpec) error {
	models, _ := json.Marshal(instanceSpec.Models)
	_, operationError := instanceStore.store.pool.Exec(operationContext, `
		INSERT INTO instances (id, workspace, models, default_model, process_limit, agent_depth_limit, created_at, stopped)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			workspace = EXCLUDED.workspace,
			models = EXCLUDED.models,
			default_model = EXCLUDED.default_model,
			process_limit = EXCLUDED.process_limit,
			agent_depth_limit = EXCLUDED.agent_depth_limit,
			stopped = EXCLUDED.stopped`,
		instanceSpec.ID, instanceSpec.Workspace, models, instanceSpec.DefaultModel, instanceSpec.ProcessLimit, instanceSpec.AgentDepthLimit, instanceSpec.CreatedAt, instanceSpec.Stopped)
	return operationError
}

func (instanceStore *instances) Get(operationContext context.Context, identifier string) (atom.InstanceSpec, error) {
	var instanceSpec atom.InstanceSpec
	var models []byte
	operationError := instanceStore.store.pool.QueryRow(operationContext, `
		SELECT id, workspace, models, default_model, process_limit, agent_depth_limit, created_at, stopped
		FROM instances WHERE id = $1`, identifier).
		Scan(&instanceSpec.ID, &instanceSpec.Workspace, &models, &instanceSpec.DefaultModel, &instanceSpec.ProcessLimit, &instanceSpec.AgentDepthLimit, &instanceSpec.CreatedAt, &instanceSpec.Stopped)
	if operationError != nil {
		return atom.InstanceSpec{}, operationError
	}
	_ = json.Unmarshal(models, &instanceSpec.Models)
	return instanceSpec, nil
}

func (instanceStore *instances) All(operationContext context.Context) ([]atom.InstanceSpec, error) {
	rows, operationError := instanceStore.store.pool.Query(operationContext, `
		SELECT id, workspace, models, default_model, process_limit, agent_depth_limit, created_at, stopped
		FROM instances ORDER BY id`)
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
	_, operationError := instanceStore.store.pool.Exec(operationContext, `DELETE FROM instances WHERE id = $1`, identifier)
	return operationError
}
