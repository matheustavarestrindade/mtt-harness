package sidekick

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const databaseSchema = `CREATE SCHEMA IF NOT EXISTS sidekick;
CREATE TABLE IF NOT EXISTS sidekick.sessions(workspace_id text NOT NULL,session_id text NOT NULL,data jsonb NOT NULL,PRIMARY KEY(workspace_id,session_id));
CREATE TABLE IF NOT EXISTS sidekick.runs(workspace_id text NOT NULL,session_id text NOT NULL,id text NOT NULL,data jsonb NOT NULL,PRIMARY KEY(workspace_id,id));
CREATE INDEX IF NOT EXISTS sidekick_run_sessions ON sidekick.runs(workspace_id,session_id);
CREATE INDEX IF NOT EXISTS sidekick_delivered_hints ON sidekick.runs(workspace_id,session_id,(data->>'hint_at') DESC,id DESC) WHERE data ? 'hint';
CREATE TABLE IF NOT EXISTS sidekick.metrics(workspace_id text NOT NULL,name text NOT NULL,value bigint NOT NULL DEFAULT 0,PRIMARY KEY(workspace_id,name));`

type postgresRepository struct{ pool *pgxpool.Pool }

func openRepository(operationContext context.Context, databaseURL string) (*postgresRepository, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("sidekick has no Postgres connection")
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
	if _, operationError = transaction.Exec(operationContext, `SELECT pg_advisory_xact_lock(71393003)`); operationError == nil {
		_, operationError = transaction.Exec(operationContext, databaseSchema)
	}
	if operationError != nil {
		transaction.Rollback(context.WithoutCancel(operationContext))
		pool.Close()
		return nil, operationError
	}
	if operationError = transaction.Commit(operationContext); operationError != nil {
		pool.Close()
		return nil, operationError
	}
	return &postgresRepository{pool: pool}, nil
}
func (database *postgresRepository) Close() { database.pool.Close() }

func (database *postgresRepository) ReadSession(operationContext context.Context, workspaceID, sessionID string) (sessionState, error) {
	var data []byte
	operationError := database.pool.QueryRow(operationContext, `SELECT data FROM sidekick.sessions WHERE workspace_id=$1 AND session_id=$2`, workspaceID, sessionID).Scan(&data)
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

func (database *postgresRepository) UpdateSession(operationContext context.Context, workspaceID, sessionID string, update func(*sessionState) (mutation, error)) (sessionState, error) {
	if workspaceID == "" || sessionID == "" {
		return sessionState{}, fmt.Errorf("sidekick storage requires workspace and session IDs")
	}
	transaction, operationError := database.pool.Begin(operationContext)
	if operationError != nil {
		return sessionState{}, operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	if _, operationError := transaction.Exec(operationContext, `SELECT pg_advisory_xact_lock(hashtextextended($1,71395))`, workspaceID+":"+sessionID); operationError != nil {
		return sessionState{}, operationError
	}
	var data []byte
	operationError = transaction.QueryRow(operationContext, `SELECT data FROM sidekick.sessions WHERE workspace_id=$1 AND session_id=$2`, workspaceID, sessionID).Scan(&data)
	if operationError != nil && !errors.Is(operationError, pgx.ErrNoRows) {
		return sessionState{}, operationError
	}
	var state sessionState
	if len(data) > 0 {
		if operationError = json.Unmarshal(data, &state); operationError != nil {
			return state, operationError
		}
	}
	var legacy []hintEvent
	if !state.ActivityRecorded && !state.Deleted {
		legacy = append(legacy, state.Events...)
		state.ActivityRecorded = true
	}
	changes, operationError := update(&state)
	if operationError != nil {
		return state, operationError
	}
	state.Revision++
	data, operationError = json.Marshal(state)
	if operationError != nil {
		return state, operationError
	}
	if _, operationError = transaction.Exec(operationContext, `INSERT INTO sidekick.sessions(workspace_id,session_id,data) VALUES($1,$2,$3) ON CONFLICT(workspace_id,session_id) DO UPDATE SET data=excluded.data`, workspaceID, sessionID, data); operationError != nil {
		return state, operationError
	}
	// Preserve already-delivered legacy hints on the next normal write, before
	// projection retirement can remove them. API reads perform no migration.
	for _, event := range legacy {
		hintData, operationError := json.Marshal(event)
		if operationError != nil {
			return state, operationError
		}
		if _, operationError = transaction.Exec(operationContext, `INSERT INTO sidekick.runs(workspace_id,session_id,id,data) VALUES($1,$2,$3,jsonb_build_object('id',$3::text,'workspace_id',$1::text,'session_id',$2::text,'status','delivered','hint',$4::jsonb,'hint_at',$5::text)) ON CONFLICT(workspace_id,id) DO UPDATE SET data=sidekick.runs.data || jsonb_build_object('hint',$4::jsonb,'hint_at',$5::text) WHERE NOT(sidekick.runs.data ? 'hint') AND sidekick.runs.session_id=$2`, workspaceID, sessionID, event.ID, hintData, event.CreatedAt.UTC().Format(activityTimeFormat)); operationError != nil {
			return state, operationError
		}
	}
	if changes.PurgeRuns {
		if _, operationError = transaction.Exec(operationContext, `DELETE FROM sidekick.runs WHERE workspace_id=$1 AND session_id=$2 AND NOT (data ? 'hint' AND COALESCE(data->'hint'->>'anchor','')=ANY(COALESCE($3::text[],'{}'::text[])))`, workspaceID, sessionID, changes.RetainHintAnchors); operationError != nil {
			return state, operationError
		}
	}
	if changes.RunID != "" {
		var hintData []byte
		hintAt := ""
		if changes.Hint != nil {
			hintData, operationError = json.Marshal(changes.Hint)
			if operationError != nil {
				return state, operationError
			}
			hintAt = changes.Hint.CreatedAt.UTC().Format(activityTimeFormat)
		}
		updated, updateError := transaction.Exec(operationContext, `UPDATE sidekick.runs SET data=jsonb_set(data,'{status}',to_jsonb($3::text)) || CASE WHEN $4::jsonb IS NULL THEN '{}'::jsonb ELSE jsonb_build_object('hint',$4::jsonb,'hint_at',$5::text) END WHERE workspace_id=$1 AND session_id=$2 AND id=$6`, workspaceID, sessionID, changes.RunStatus, hintData, hintAt, changes.RunID)
		if updateError != nil {
			return state, updateError
		}
		if changes.Hint != nil && updated.RowsAffected() != 1 {
			return state, fmt.Errorf("sidekick hint run is unavailable")
		}
	}
	if changes.Run != nil {
		data, operationError = json.Marshal(changes.Run)
		if operationError != nil {
			return state, operationError
		}
		if _, operationError = transaction.Exec(operationContext, `INSERT INTO sidekick.runs(workspace_id,session_id,id,data) VALUES($1,$2,$3,$4) ON CONFLICT(workspace_id,id) DO UPDATE SET data=excluded.data`, workspaceID, sessionID, changes.Run.ID, data); operationError != nil {
			return state, operationError
		}
	}
	for name, value := range changes.Counters {
		if _, operationError = transaction.Exec(operationContext, `INSERT INTO sidekick.metrics(workspace_id,name,value) VALUES($1,$2,$3) ON CONFLICT(workspace_id,name) DO UPDATE SET value=sidekick.metrics.value+excluded.value`, workspaceID, name, value); operationError != nil {
			return state, operationError
		}
	}
	return state, transaction.Commit(operationContext)
}

func (database *postgresRepository) Counters(operationContext context.Context, workspaceID string) (map[string]int64, error) {
	result := map[string]int64{}
	rows, operationError := database.pool.Query(operationContext, `SELECT name,value FROM sidekick.metrics WHERE workspace_id=$1`, workspaceID)
	if operationError != nil {
		return nil, operationError
	}
	for rows.Next() {
		var name string
		var value int64
		if operationError = rows.Scan(&name, &value); operationError != nil {
			rows.Close()
			return nil, operationError
		}
		result[name] = value
	}
	rows.Close()
	if operationError = rows.Err(); operationError != nil {
		return nil, operationError
	}
	rows, operationError = database.pool.Query(operationContext, `SELECT CASE WHEN data->>'status'='running' AND (data->>'expires_at')::timestamptz<now() THEN 'interrupted' ELSE data->>'status' END AS status,count(*) FROM sidekick.runs WHERE workspace_id=$1 GROUP BY status`, workspaceID)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if operationError = rows.Scan(&status, &count); operationError != nil {
			return nil, operationError
		}
		result["runs/"+status] = count
	}
	return result, rows.Err()
}
