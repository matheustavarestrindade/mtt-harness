package postgres

import (
	"context"
	"encoding/json"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type processes struct{ store *Store }

func (processStore *processes) Save(operationContext context.Context, record atom.ProcessRecord) error {
	specification, _ := json.Marshal(record.Spec)
	var exit any
	if record.Exit != nil {
		data, _ := json.Marshal(record.Exit)
		exit = data
	}
	_, operationError := processStore.store.pool.Exec(operationContext, `
		INSERT INTO processes (id, instance_id, session_id, spec, pid, status, exit, started_at, ended_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			pid = EXCLUDED.pid,
			status = EXCLUDED.status,
			exit = EXCLUDED.exit,
			ended_at = EXCLUDED.ended_at`,
		record.ID, record.InstanceID, string(record.SessionID), specification, record.PID, record.Status, exit, record.StartedAt, record.EndedAt)
	return operationError
}

func (processStore *processes) Get(operationContext context.Context, identifier string) (atom.ProcessRecord, error) {
	var record atom.ProcessRecord
	var specification, exit []byte
	operationError := processStore.store.pool.QueryRow(operationContext, `
		SELECT id, instance_id, session_id, spec, pid, status, exit, started_at, ended_at
		FROM processes WHERE id = $1`, identifier).
		Scan(&record.ID, &record.InstanceID, &record.SessionID, &specification, &record.PID, &record.Status, &exit, &record.StartedAt, &record.EndedAt)
	if operationError != nil {
		return atom.ProcessRecord{}, operationError
	}
	_ = json.Unmarshal(specification, &record.Spec)
	if len(exit) > 0 {
		_ = json.Unmarshal(exit, &record.Exit)
	}
	return record, nil
}

func (processStore *processes) CountRunning(operationContext context.Context, instanceID string) (int, error) {
	var count int
	operationError := processStore.store.pool.QueryRow(operationContext, `SELECT COUNT(*) FROM processes WHERE instance_id = $1 AND status = 'running'`, instanceID).Scan(&count)
	return count, operationError
}

func (processStore *processes) List(operationContext context.Context, session atom.SessionID) ([]atom.ProcessRecord, error) {
	rows, operationError := processStore.store.pool.Query(operationContext, `
		SELECT id, instance_id, session_id, spec, pid, status, exit, started_at, ended_at
		FROM processes WHERE session_id = $1 ORDER BY started_at`, string(session))
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var list []atom.ProcessRecord
	for rows.Next() {
		var record atom.ProcessRecord
		var specification, exit []byte
		if operationError := rows.Scan(&record.ID, &record.InstanceID, &record.SessionID, &specification, &record.PID, &record.Status, &exit, &record.StartedAt, &record.EndedAt); operationError != nil {
			return nil, operationError
		}
		_ = json.Unmarshal(specification, &record.Spec)
		if len(exit) > 0 {
			_ = json.Unmarshal(exit, &record.Exit)
		}
		list = append(list, record)
	}
	return list, rows.Err()
}
