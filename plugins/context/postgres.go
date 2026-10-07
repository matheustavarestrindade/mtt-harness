package contextplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct{ pool *pgxpool.Pool }

func openRepository(operationContext context.Context, databaseURL string) (*postgresRepository, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("context plugin has no Postgres connection")
	}
	configuration, operationError := pgxpool.ParseConfig(databaseURL)
	if operationError != nil {
		return nil, operationError
	}
	configuration.MaxConns = 6
	pool, operationError := pgxpool.NewWithConfig(operationContext, configuration)
	if operationError != nil {
		return nil, operationError
	}
	schemaTransaction, operationError := pool.Begin(operationContext)
	if operationError != nil {
		pool.Close()
		return nil, operationError
	}
	defer schemaTransaction.Rollback(context.WithoutCancel(operationContext))
	if _, operationError := schemaTransaction.Exec(operationContext, `SELECT pg_advisory_xact_lock(71393001)`); operationError != nil {
		schemaTransaction.Rollback(context.WithoutCancel(operationContext))
		pool.Close()
		return nil, operationError
	}
	if _, operationError := schemaTransaction.Exec(operationContext, databaseSchema); operationError != nil {
		schemaTransaction.Rollback(context.WithoutCancel(operationContext))
		pool.Close()
		return nil, fmt.Errorf("context plugin schema: %w", operationError)
	}
	if operationError := schemaTransaction.Commit(operationContext); operationError != nil {
		pool.Close()
		return nil, operationError
	}
	return &postgresRepository{pool: pool}, nil
}

func (database *postgresRepository) Close() { database.pool.Close() }

func (database *postgresRepository) Read(operationContext context.Context, workspaceID, kind, identifier string) (json.RawMessage, error) {
	var document json.RawMessage
	operationError := database.pool.QueryRow(operationContext, `SELECT data FROM context_plugin.documents WHERE workspace_id=$1 AND kind=$2 AND key=$3`, workspaceID, kind, identifier).Scan(&document)
	if errors.Is(operationError, pgx.ErrNoRows) {
		return nil, nil
	}
	return document, operationError
}

func (database *postgresRepository) List(operationContext context.Context, workspaceID, kind, after string, limit int) ([]json.RawMessage, error) {
	rows, operationError := database.pool.Query(operationContext, `SELECT data FROM context_plugin.documents WHERE workspace_id=$1 AND kind=$2 AND key>$3 ORDER BY key LIMIT $4`, workspaceID, kind, after, limit)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var documents []json.RawMessage
	for rows.Next() {
		var document json.RawMessage
		if operationError := rows.Scan(&document); operationError != nil {
			return nil, operationError
		}
		documents = append(documents, document)
	}
	return documents, rows.Err()
}

func (database *postgresRepository) Transact(operationContext context.Context, workspaceID string, operation func(transaction) error) error {
	if workspaceID == "" {
		return fmt.Errorf("memory transaction requires a workspace ID")
	}
	operationTransaction, operationError := database.pool.Begin(operationContext)
	if operationError != nil {
		return operationError
	}
	defer operationTransaction.Rollback(context.WithoutCancel(operationContext))
	// A workspace lock serializes short metadata commits across processes. Model
	// calls and embedding work always finish before entering this transaction.
	if _, operationError = operationTransaction.Exec(operationContext, `SELECT pg_advisory_xact_lock(hashtextextended($1,71393))`, workspaceID); operationError != nil {
		return operationError
	}
	if operationError = operation(&postgresTransaction{operationContext: operationContext, transaction: operationTransaction, workspaceID: workspaceID}); operationError != nil {
		return operationError
	}
	return operationTransaction.Commit(operationContext)
}

func (database *postgresRepository) PendingWorkspaces(operationContext context.Context, limit int) ([]string, error) {
	rows, operationError := database.pool.Query(operationContext, `SELECT workspace_id FROM context_plugin.documents WHERE (kind='job' AND data->>'status' IN ('pending','running')) OR kind='workspace' GROUP BY workspace_id ORDER BY workspace_id LIMIT $1`, limit)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	var identifiers []string
	for rows.Next() {
		var identifier string
		if operationError := rows.Scan(&identifier); operationError != nil {
			return nil, operationError
		}
		identifiers = append(identifiers, identifier)
	}
	return identifiers, rows.Err()
}

func (database *postgresRepository) Count(operationContext context.Context, workspaceID string) (map[string]int64, error) {
	rows, operationError := database.pool.Query(operationContext, `SELECT kind,COALESCE(data->>'status',''),COUNT(*) FROM context_plugin.documents WHERE workspace_id=$1 GROUP BY kind,data->>'status'`, workspaceID)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	result := map[string]int64{}
	for rows.Next() {
		var kind, status string
		var count int64
		if operationError := rows.Scan(&kind, &status, &count); operationError != nil {
			return nil, operationError
		}
		result[kind+"/"+status] = count
	}
	if operationError := rows.Err(); operationError != nil {
		return nil, operationError
	}
	rows.Close()
	metrics, operationError := database.pool.Query(operationContext, `SELECT agent,name,value,duration_ms FROM context_plugin.metrics WHERE workspace_id=$1`, workspaceID)
	if operationError != nil {
		return nil, operationError
	}
	defer metrics.Close()
	for metrics.Next() {
		var agent, name string
		var value, duration int64
		if operationError := metrics.Scan(&agent, &name, &value, &duration); operationError != nil {
			return nil, operationError
		}
		result[agent+"/"+name] = value
		result[agent+"/"+name+"/duration_ms"] = duration
	}
	return result, metrics.Err()
}

func (database *postgresRepository) Metric(operationContext context.Context, workspaceID, agent, name string, value, duration int64) error {
	_, operationError := database.pool.Exec(operationContext, `INSERT INTO context_plugin.metrics(workspace_id,agent,name,value,duration_ms) VALUES($1,$2,$3,$4,$5) ON CONFLICT(workspace_id,agent,name) DO UPDATE SET value=context_plugin.metrics.value+EXCLUDED.value,duration_ms=context_plugin.metrics.duration_ms+EXCLUDED.duration_ms,updated_at=now()`, workspaceID, agent, name, value, duration)
	return operationError
}
