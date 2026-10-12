package contextplugin

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// A model/dimension expression index preserves existing vector spaces. Larger
// vectors use bounded exact nearest-neighbor retrieval because pgvector's
// vector HNSW operator class supports at most 2000 dimensions.
func (database *postgresRepository) ensureSearchIndex(operationContext context.Context, model string, dimensions int) error {
	if dimensions == 0 || dimensions > 2000 {
		return nil
	}
	digest := sha256.Sum256([]byte(model))
	name := fmt.Sprintf("context_nearest_%d_%x", dimensions, digest[:12])
	if _, found := database.searchIndexes.Load(name); found {
		return nil
	}
	transaction, operationError := database.pool.Begin(operationContext)
	if operationError != nil {
		return operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	if _, operationError = transaction.Exec(operationContext, `SELECT pg_advisory_xact_lock(hashtextextended($1,71393004))`, name); operationError != nil {
		return operationError
	}
	// Simple protocol safely quotes the configured model value in DDL. Integer
	// dimensions and the generated identifier never come from free-form SQL.
	statement := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON context_plugin.vectors USING hnsw ((embedding::vector(%d)) vector_cosine_ops) WHERE model=$1 AND vector_dims(embedding)=%d`, pgx.Identifier{name}.Sanitize(), dimensions, dimensions)
	if _, operationError = transaction.Exec(operationContext, statement, pgx.QueryExecModeSimpleProtocol, model); operationError != nil {
		return fmt.Errorf("memory search index: %w", operationError)
	}
	if operationError = transaction.Commit(operationContext); operationError != nil {
		return operationError
	}
	database.searchIndexes.Store(name, struct{}{})
	return nil
}
