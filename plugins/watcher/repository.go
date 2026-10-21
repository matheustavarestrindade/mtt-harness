package watcher

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type repository interface {
	Close()
	Session(context.Context, string, string) (sessionState, error)
	UpdateSession(context.Context, string, string, func(*sessionState) error) error
	SaveRun(context.Context, runRecord) error
	Runs(context.Context, string, string, string, int) (runPage, error)
	Activity(context.Context, string, string, string, int) (runPage, sessionState, error)
	Counters(context.Context, string) (map[string]int64, error)
	RetireHistory(context.Context, string, string, bool, []string) error
}
type postgresRepository struct{ pool *pgxpool.Pool }

const databaseSchema = `CREATE SCHEMA IF NOT EXISTS watcher;
CREATE TABLE IF NOT EXISTS watcher.sessions(workspace_id text NOT NULL,session_id text NOT NULL,data jsonb NOT NULL,PRIMARY KEY(workspace_id,session_id));
CREATE TABLE IF NOT EXISTS watcher.runs(workspace_id text NOT NULL,session_id text NOT NULL,id text NOT NULL,created_at timestamptz NOT NULL,status text NOT NULL,data jsonb NOT NULL,PRIMARY KEY(workspace_id,id));
CREATE INDEX IF NOT EXISTS watcher_runs_session ON watcher.runs(workspace_id,session_id,created_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS watcher_runs_workspace ON watcher.runs(workspace_id,created_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS watcher_runs_state ON watcher.runs(workspace_id,status);
CREATE TABLE IF NOT EXISTS watcher.metrics(workspace_id text NOT NULL,name text NOT NULL,value bigint NOT NULL DEFAULT 0,PRIMARY KEY(workspace_id,name));`

func openRepository(operationContext context.Context, databaseURL string) (*postgresRepository, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("watcher has no Postgres connection")
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
	if _, operationError = transaction.Exec(operationContext, `SELECT pg_advisory_xact_lock(71393005)`); operationError == nil {
		_, operationError = transaction.Exec(operationContext, databaseSchema)
	}
	if operationError == nil {
		_, operationError = transaction.Exec(operationContext, `UPDATE watcher.runs SET status='interrupted',data=data||jsonb_build_object('status','interrupted','error','harness stopped before the review completed') WHERE status IN ('queued','running')`)
	}
	if operationError == nil {
		operationError = transaction.Commit(operationContext)
	}
	if operationError != nil {
		pool.Close()
		return nil, operationError
	}
	return &postgresRepository{pool: pool}, nil
}
func (database *postgresRepository) Close() { database.pool.Close() }
func decodeSession(data []byte) (sessionState, error) {
	state := sessionState{Schedules: map[string]schedule{}}
	if len(data) > 0 {
		if operationError := json.Unmarshal(data, &state); operationError != nil {
			return state, operationError
		}
	}
	if state.Schedules == nil {
		state.Schedules = map[string]schedule{}
	}
	return state, nil
}
func (database *postgresRepository) Session(operationContext context.Context, workspaceID, sessionID string) (sessionState, error) {
	var data []byte
	operationError := database.pool.QueryRow(operationContext, `SELECT data FROM watcher.sessions WHERE workspace_id=$1 AND session_id=$2`, workspaceID, sessionID).Scan(&data)
	if operationError != nil && !errors.Is(operationError, pgx.ErrNoRows) {
		return sessionState{}, operationError
	}
	return decodeSession(data)
}
func lockedSession(operationContext context.Context, transaction pgx.Tx, workspaceID, sessionID string) (sessionState, error) {
	if workspaceID == "" || sessionID == "" {
		return sessionState{}, fmt.Errorf("watcher storage requires workspace and session IDs")
	}
	if _, operationError := transaction.Exec(operationContext, `SELECT pg_advisory_xact_lock(hashtextextended($1,71396))`, workspaceID+":"+sessionID); operationError != nil {
		return sessionState{}, operationError
	}
	var data []byte
	operationError := transaction.QueryRow(operationContext, `SELECT data FROM watcher.sessions WHERE workspace_id=$1 AND session_id=$2 FOR UPDATE`, workspaceID, sessionID).Scan(&data)
	if operationError != nil && !errors.Is(operationError, pgx.ErrNoRows) {
		return sessionState{}, operationError
	}
	return decodeSession(data)
}
func saveSession(operationContext context.Context, transaction pgx.Tx, workspaceID, sessionID string, state sessionState) error {
	state.Revision++
	data, operationError := json.Marshal(state)
	if operationError != nil {
		return operationError
	}
	_, operationError = transaction.Exec(operationContext, `INSERT INTO watcher.sessions(workspace_id,session_id,data) VALUES($1,$2,$3) ON CONFLICT(workspace_id,session_id) DO UPDATE SET data=excluded.data`, workspaceID, sessionID, data)
	return operationError
}
func (database *postgresRepository) UpdateSession(operationContext context.Context, workspaceID, sessionID string, update func(*sessionState) error) error {
	transaction, operationError := database.pool.Begin(operationContext)
	if operationError != nil {
		return operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	state, operationError := lockedSession(operationContext, transaction, workspaceID, sessionID)
	if operationError != nil {
		return operationError
	}
	if state.Deleted {
		return fmt.Errorf("watcher session is deleted")
	}
	if operationError = update(&state); operationError != nil {
		return operationError
	}
	if operationError = saveSession(operationContext, transaction, workspaceID, sessionID, state); operationError != nil {
		return operationError
	}
	return transaction.Commit(operationContext)
}
func (database *postgresRepository) SaveRun(operationContext context.Context, run runRecord) error {
	transaction, operationError := database.pool.Begin(operationContext)
	if operationError != nil {
		return operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	state, operationError := lockedSession(operationContext, transaction, run.WorkspaceID, string(run.SessionID))
	if operationError != nil {
		return operationError
	}
	if state.Deleted {
		return fmt.Errorf("watcher session is deleted")
	}
	var previous string
	operationError = transaction.QueryRow(operationContext, `SELECT status FROM watcher.runs WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, run.WorkspaceID, run.ID).Scan(&previous)
	if operationError != nil && !errors.Is(operationError, pgx.ErrNoRows) {
		return operationError
	}
	data, operationError := json.Marshal(run)
	if operationError != nil {
		return operationError
	}
	if _, operationError = transaction.Exec(operationContext, `INSERT INTO watcher.runs(workspace_id,session_id,id,created_at,status,data) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(workspace_id,id) DO UPDATE SET status=excluded.status,data=excluded.data WHERE watcher.runs.session_id=excluded.session_id`, run.WorkspaceID, string(run.SessionID), run.ID, run.CreatedAt, run.Status, data); operationError != nil {
		return operationError
	}
	if previous != run.Status {
		if _, operationError = transaction.Exec(operationContext, `INSERT INTO watcher.metrics(workspace_id,name,value) VALUES($1,$2,1) ON CONFLICT(workspace_id,name) DO UPDATE SET value=watcher.metrics.value+1`, run.WorkspaceID, run.Status); operationError != nil {
			return operationError
		}
	}
	if operationError = saveSession(operationContext, transaction, run.WorkspaceID, string(run.SessionID), state); operationError != nil {
		return operationError
	}
	return transaction.Commit(operationContext)
}

type activityCursor struct {
	Workspace    string
	Session      string
	Time         time.Time
	ID           string
	HistoryEpoch int64
}

func (database *postgresRepository) Runs(operationContext context.Context, workspaceID, sessionID, cursor string, limit int) (runPage, error) {
	if sessionID != "" {
		page, _, operationError := database.Activity(operationContext, workspaceID, sessionID, cursor, limit)
		return page, operationError
	}
	return readRunPage(operationContext, database.pool, workspaceID, sessionID, cursor, limit, 0)
}

type runQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readRunPage(operationContext context.Context, query runQuerier, workspaceID, sessionID, cursor string, limit int, historyEpoch int64) (runPage, error) {
	if limit < 1 || limit > 100 {
		return runPage{}, fmt.Errorf("watcher activity limit must be 1-100")
	}
	position := activityCursor{Workspace: workspaceID, Session: sessionID, Time: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), ID: "~", HistoryEpoch: historyEpoch}
	if cursor != "" {
		if len(cursor) > 2048 {
			return runPage{}, fmt.Errorf("watcher activity cursor is invalid")
		}
		data, operationError := base64.RawURLEncoding.DecodeString(cursor)
		if operationError != nil || json.Unmarshal(data, &position) != nil || position.Workspace != workspaceID || position.Session != sessionID {
			return runPage{}, fmt.Errorf("watcher activity cursor is invalid")
		}
		if position.HistoryEpoch != historyEpoch {
			return runPage{}, harness.ErrPluginActivityChanged
		}
	}
	rows, operationError := query.Query(operationContext, `SELECT data FROM watcher.runs WHERE workspace_id=$1 AND ($2='' OR session_id=$2) AND (created_at,id)<($3,$4) ORDER BY created_at DESC,id DESC LIMIT $5`, workspaceID, sessionID, position.Time, position.ID, limit+1)
	if operationError != nil {
		return runPage{}, operationError
	}
	defer rows.Close()
	page := runPage{Runs: []runRecord{}}
	for rows.Next() {
		var data []byte
		if operationError = rows.Scan(&data); operationError != nil {
			return page, operationError
		}
		var run runRecord
		if operationError = json.Unmarshal(data, &run); operationError != nil {
			return page, operationError
		}
		page.Runs = append(page.Runs, run)
	}
	if operationError = rows.Err(); operationError != nil {
		return page, operationError
	}
	if len(page.Runs) > limit {
		page.Runs = page.Runs[:limit]
		last := page.Runs[len(page.Runs)-1]
		data, _ := json.Marshal(activityCursor{Workspace: workspaceID, Session: sessionID, Time: last.CreatedAt, ID: last.ID, HistoryEpoch: historyEpoch})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(data)
	}
	return page, nil
}

func (database *postgresRepository) Activity(operationContext context.Context, workspaceID, sessionID, cursor string, limit int) (runPage, sessionState, error) {
	transaction, operationError := database.pool.BeginTx(operationContext, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if operationError != nil {
		return runPage{}, sessionState{}, operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	var data []byte
	operationError = transaction.QueryRow(operationContext, `SELECT data FROM watcher.sessions WHERE workspace_id=$1 AND session_id=$2`, workspaceID, sessionID).Scan(&data)
	if operationError != nil && !errors.Is(operationError, pgx.ErrNoRows) {
		return runPage{}, sessionState{}, operationError
	}
	state, operationError := decodeSession(data)
	if operationError != nil {
		return runPage{}, state, operationError
	}
	page, operationError := readRunPage(operationContext, transaction, workspaceID, sessionID, cursor, limit, state.HistoryEpoch)
	if operationError != nil {
		return page, state, operationError
	}
	return page, state, transaction.Commit(operationContext)
}
func (database *postgresRepository) Counters(operationContext context.Context, workspaceID string) (map[string]int64, error) {
	rows, operationError := database.pool.Query(operationContext, `SELECT name,value FROM watcher.metrics WHERE workspace_id=$1`, workspaceID)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	counts := map[string]int64{}
	for rows.Next() {
		var name string
		var value int64
		if operationError = rows.Scan(&name, &value); operationError != nil {
			return nil, operationError
		}
		counts[name] = value
	}
	return counts, rows.Err()
}
func (database *postgresRepository) RetireHistory(operationContext context.Context, workspaceID, sessionID string, deleted bool, anchors []string) error {
	transaction, operationError := database.pool.Begin(operationContext)
	if operationError != nil {
		return operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	state, operationError := lockedSession(operationContext, transaction, workspaceID, sessionID)
	if operationError != nil {
		return operationError
	}
	state.Deleted = deleted
	state.HistoryEpoch++
	state.Schedules = map[string]schedule{}
	state.WaitingForUser = false
	state.WaitingUserID = ""
	retainedNotices := state.Notices[:0]
	for _, notice := range state.Notices {
		if !deleted && slices.Contains(anchors, notice.AnchorID) {
			retainedNotices = append(retainedNotices, notice)
		}
	}
	state.Notices = retainedNotices
	if _, operationError = transaction.Exec(operationContext, `DELETE FROM watcher.runs WHERE workspace_id=$1 AND session_id=$2 AND ($3 OR NOT(COALESCE(data->>'anchor_id','')=ANY($4::text[])))`, workspaceID, sessionID, deleted, anchors); operationError != nil {
		return operationError
	}
	if operationError = saveSession(operationContext, transaction, workspaceID, sessionID, state); operationError != nil {
		return operationError
	}
	return transaction.Commit(operationContext)
}
