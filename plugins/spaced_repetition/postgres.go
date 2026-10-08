package spacedrepetition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const databaseSchema = `
CREATE SCHEMA IF NOT EXISTS spaced_repetition;
CREATE TABLE IF NOT EXISTS spaced_repetition.sessions (
 workspace_id text NOT NULL, session_id text NOT NULL, data jsonb NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(workspace_id,session_id)
);
CREATE TABLE IF NOT EXISTS spaced_repetition.recoveries (
 workspace_id text NOT NULL, session_id text NOT NULL, id text NOT NULL,
 data jsonb NOT NULL, PRIMARY KEY(workspace_id,id)
);
CREATE INDEX IF NOT EXISTS repetition_recovery_sessions ON spaced_repetition.recoveries(workspace_id,session_id);
CREATE TABLE IF NOT EXISTS spaced_repetition.metrics (
 workspace_id text NOT NULL, name text NOT NULL, value bigint NOT NULL DEFAULT 0,
 PRIMARY KEY(workspace_id,name)
);`

type postgresRepository struct{ pool *pgxpool.Pool }

func openRepository(operationContext context.Context, databaseURL string) (*postgresRepository, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("spaced repetition has no Postgres connection")
	}
	configuration, operationError := pgxpool.ParseConfig(databaseURL)
	if operationError != nil {
		return nil, operationError
	}
	configuration.MaxConns = 4
	pool, operationError := pgxpool.NewWithConfig(operationContext, configuration)
	if operationError != nil {
		return nil, operationError
	}
	transaction, operationError := pool.Begin(operationContext)
	if operationError != nil {
		pool.Close()
		return nil, operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	if _, operationError := transaction.Exec(operationContext, `SELECT pg_advisory_xact_lock(71393002)`); operationError != nil {
		transaction.Rollback(context.WithoutCancel(operationContext))
		pool.Close()
		return nil, operationError
	}
	if _, operationError := transaction.Exec(operationContext, databaseSchema); operationError != nil {
		transaction.Rollback(context.WithoutCancel(operationContext))
		pool.Close()
		return nil, operationError
	}
	if operationError := transaction.Commit(operationContext); operationError != nil {
		pool.Close()
		return nil, operationError
	}
	return &postgresRepository{pool: pool}, nil
}

func (database *postgresRepository) Close() { database.pool.Close() }

func (database *postgresRepository) ReadSession(operationContext context.Context, workspaceID, sessionID string) (sessionState, error) {
	var data []byte
	operationError := database.pool.QueryRow(operationContext, `SELECT data FROM spaced_repetition.sessions WHERE workspace_id=$1 AND session_id=$2`, workspaceID, sessionID).Scan(&data)
	if errors.Is(operationError, pgx.ErrNoRows) {
		return sessionState{}, nil
	}
	if operationError != nil {
		return sessionState{}, operationError
	}
	var state sessionState
	operationError = json.Unmarshal(data, &state)
	return state, operationError
}

func (database *postgresRepository) UpdateSession(operationContext context.Context, workspaceID, sessionID string, update func(*sessionState) (map[string]int64, error)) (sessionState, error) {
	return database.mutateSession(operationContext, workspaceID, sessionID, update, nil)
}

func (database *postgresRepository) mutateSession(operationContext context.Context, workspaceID, sessionID string, update func(*sessionState) (map[string]int64, error), run *recoveryRun) (sessionState, error) {
	if workspaceID == "" || sessionID == "" {
		return sessionState{}, fmt.Errorf("reminder storage requires workspace and session IDs")
	}
	transaction, operationError := database.pool.Begin(operationContext)
	if operationError != nil {
		return sessionState{}, operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	if _, operationError := transaction.Exec(operationContext, `SELECT pg_advisory_xact_lock(hashtextextended($1,71394))`, workspaceID+":"+sessionID); operationError != nil {
		return sessionState{}, operationError
	}
	var data []byte
	operationError = transaction.QueryRow(operationContext, `SELECT data FROM spaced_repetition.sessions WHERE workspace_id=$1 AND session_id=$2`, workspaceID, sessionID).Scan(&data)
	if operationError != nil && !errors.Is(operationError, pgx.ErrNoRows) {
		return sessionState{}, operationError
	}
	var state sessionState
	if len(data) > 0 {
		if operationError := json.Unmarshal(data, &state); operationError != nil {
			return state, operationError
		}
	}
	counters, operationError := update(&state)
	if operationError != nil {
		return state, operationError
	}
	state.Revision++
	data, operationError = json.Marshal(state)
	if operationError != nil {
		return state, operationError
	}
	if _, operationError := transaction.Exec(operationContext, `INSERT INTO spaced_repetition.sessions(workspace_id,session_id,data) VALUES($1,$2,$3) ON CONFLICT(workspace_id,session_id) DO UPDATE SET data=excluded.data,updated_at=now()`, workspaceID, sessionID, data); operationError != nil {
		return state, operationError
	}
	for name, value := range counters {
		if _, operationError := transaction.Exec(operationContext, `INSERT INTO spaced_repetition.metrics(workspace_id,name,value) VALUES($1,$2,$3) ON CONFLICT(workspace_id,name) DO UPDATE SET value=spaced_repetition.metrics.value+excluded.value`, workspaceID, name, value); operationError != nil {
			return state, operationError
		}
	}
	if run != nil {
		data, operationError := json.Marshal(run)
		if operationError != nil {
			return state, operationError
		}
		if _, operationError := transaction.Exec(operationContext, `INSERT INTO spaced_repetition.recoveries(workspace_id,session_id,id,data) VALUES($1,$2,$3,$4) ON CONFLICT(workspace_id,id) DO UPDATE SET data=excluded.data`, run.WorkspaceID, run.SessionID, run.ID, data); operationError != nil {
			return state, operationError
		}
	}
	return state, transaction.Commit(operationContext)
}

func (database *postgresRepository) CompleteRecovery(operationContext context.Context, run recoveryRun, report recoveryReport, counters map[string]int64) error {
	_, operationError := database.mutateSession(operationContext, run.WorkspaceID, run.SessionID, func(state *sessionState) (map[string]int64, error) {
		if state.Deleted || state.Fenced || state.Epoch != run.Epoch || state.SourceUser != run.SourceUser {
			return nil, errSessionRetired
		}
		state.Pending = &report
		return counters, nil
	}, &run)
	return operationError
}

func (database *postgresRepository) ReadRecovery(operationContext context.Context, workspaceID, identifier string) (recoveryRun, error) {
	var data []byte
	operationError := database.pool.QueryRow(operationContext, `SELECT data FROM spaced_repetition.recoveries WHERE workspace_id=$1 AND id=$2`, workspaceID, identifier).Scan(&data)
	if errors.Is(operationError, pgx.ErrNoRows) {
		return recoveryRun{}, nil
	}
	var run recoveryRun
	if operationError != nil {
		return run, operationError
	}
	operationError = json.Unmarshal(data, &run)
	return run, operationError
}

func (database *postgresRepository) SaveRecovery(operationContext context.Context, run recoveryRun) error {
	transaction, operationError := database.pool.Begin(operationContext)
	if operationError != nil {
		return operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	if _, operationError := transaction.Exec(operationContext, `SELECT pg_advisory_xact_lock(hashtextextended($1,71394))`, run.WorkspaceID+":"+run.SessionID); operationError != nil {
		return operationError
	}
	var deleted bool
	operationError = transaction.QueryRow(operationContext, `SELECT COALESCE((data->>'deleted')::boolean,false) OR COALESCE((data->>'fenced')::boolean,false) OR COALESCE((data->>'epoch')::bigint,0)<>$3 FROM spaced_repetition.sessions WHERE workspace_id=$1 AND session_id=$2`, run.WorkspaceID, run.SessionID, run.Epoch).Scan(&deleted)
	if operationError != nil && !errors.Is(operationError, pgx.ErrNoRows) {
		return operationError
	}
	if deleted {
		return errSessionRetired
	}
	data, operationError := json.Marshal(run)
	if operationError != nil {
		return operationError
	}
	if _, operationError := transaction.Exec(operationContext, `INSERT INTO spaced_repetition.recoveries(workspace_id,session_id,id,data) VALUES($1,$2,$3,$4) ON CONFLICT(workspace_id,id) DO UPDATE SET data=excluded.data`, run.WorkspaceID, run.SessionID, run.ID, data); operationError != nil {
		return operationError
	}
	return transaction.Commit(operationContext)
}

func (database *postgresRepository) DeleteRecoveries(operationContext context.Context, workspaceID, sessionID string) error {
	_, operationError := database.pool.Exec(operationContext, `DELETE FROM spaced_repetition.recoveries WHERE workspace_id=$1 AND session_id=$2`, workspaceID, sessionID)
	return operationError
}

func (database *postgresRepository) Counters(operationContext context.Context, workspaceID string) (map[string]int64, error) {
	result := map[string]int64{}
	rows, operationError := database.pool.Query(operationContext, `SELECT name,value FROM spaced_repetition.metrics WHERE workspace_id=$1`, workspaceID)
	if operationError != nil {
		return nil, operationError
	}
	for rows.Next() {
		var name string
		var value int64
		if operationError := rows.Scan(&name, &value); operationError != nil {
			rows.Close()
			return nil, operationError
		}
		result[name] = value
	}
	rows.Close()
	if operationError := rows.Err(); operationError != nil {
		return nil, operationError
	}
	rows, operationError = database.pool.Query(operationContext, `SELECT CASE WHEN data->>'status'='running' AND (data->>'expires_at')::timestamptz<now() THEN 'interrupted' ELSE data->>'status' END AS status,count(*) FROM spaced_repetition.recoveries WHERE workspace_id=$1 GROUP BY status`, workspaceID)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if operationError := rows.Scan(&status, &count); operationError != nil {
			return nil, operationError
		}
		result["recovery/"+status] = count
	}
	return result, rows.Err()
}
