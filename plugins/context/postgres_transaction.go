package contextplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

type postgresTransaction struct {
	operationContext context.Context
	transaction      pgx.Tx
	workspaceID      string
}

func (database *postgresTransaction) AddMetric(agent, name string, value, duration int64) error {
	_, operationError := database.transaction.Exec(database.operationContext, `INSERT INTO context_plugin.metrics(workspace_id,agent,name,value,duration_ms) VALUES($1,$2,$3,$4,$5) ON CONFLICT(workspace_id,agent,name) DO UPDATE SET value=context_plugin.metrics.value+EXCLUDED.value,duration_ms=context_plugin.metrics.duration_ms+EXCLUDED.duration_ms,updated_at=now()`, database.workspaceID, agent, name, value, duration)
	return operationError
}

func (database *postgresTransaction) Get(kind, identifier string) (json.RawMessage, error) {
	var document json.RawMessage
	operationError := database.transaction.QueryRow(database.operationContext, `SELECT data FROM context_plugin.documents WHERE workspace_id=$1 AND kind=$2 AND key=$3`, database.workspaceID, kind, identifier).Scan(&document)
	if errors.Is(operationError, pgx.ErrNoRows) {
		return nil, nil
	}
	return document, operationError
}

func (database *postgresTransaction) List(kind string) ([]json.RawMessage, error) {
	rows, operationError := database.transaction.Query(database.operationContext, `SELECT data FROM context_plugin.documents WHERE workspace_id=$1 AND kind=$2 ORDER BY key`, database.workspaceID, kind)
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

func (database *postgresTransaction) Put(kind, identifier string, value any) error {
	data, operationError := json.Marshal(value)
	if operationError != nil {
		return operationError
	}
	_, operationError = database.transaction.Exec(database.operationContext, `INSERT INTO context_plugin.documents(workspace_id,kind,key,data) VALUES($1,$2,$3,$4) ON CONFLICT(workspace_id,kind,key) DO UPDATE SET data=EXCLUDED.data,updated_at=now()`, database.workspaceID, kind, identifier, data)
	return operationError
}

func (database *postgresTransaction) NextSourceID() (int64, error) {
	var identifier int64
	operationError := database.transaction.QueryRow(database.operationContext, `SELECT nextval('context_plugin.source_ids')`).Scan(&identifier)
	return identifier, operationError
}

func (database *postgresTransaction) ReplaceVectors(kind, identifier string, version int, model string, categories []string, chunks []vectorChunk, deleted, historical bool) error {
	if _, operationError := database.transaction.Exec(database.operationContext, `DELETE FROM context_plugin.vectors WHERE workspace_id=$1 AND kind=$2 AND key=$3 AND version=$4`, database.workspaceID, kind, identifier, version); operationError != nil {
		return operationError
	}
	for position, chunk := range chunks {
		vector, operationError := encodeVector(chunk.Vector)
		if operationError != nil {
			return operationError
		}
		if categories == nil {
			categories = []string{}
		}
		_, operationError = database.transaction.Exec(database.operationContext, `INSERT INTO context_plugin.vectors(workspace_id,kind,key,version,chunk,model,categories,content,embedding,deleted,historical) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::vector,$10,$11)`, database.workspaceID, kind, identifier, version, position, model, categories, chunk.Text, vector, deleted, historical)
		if operationError != nil {
			return operationError
		}
	}
	return nil
}

func (database *postgresTransaction) MarkVectors(kind, identifier string, deleted, historical bool) error {
	_, operationError := database.transaction.Exec(database.operationContext, `UPDATE context_plugin.vectors SET deleted=$4,historical=$5 WHERE workspace_id=$1 AND kind=$2 AND key=$3`, database.workspaceID, kind, identifier, deleted, historical)
	return operationError
}

func encodeVector(values []float64) (string, error) {
	if len(values) == 0 || len(values) > 4096 {
		return "", fmt.Errorf("embedding dimensions %d are outside 1..4096", len(values))
	}
	var encoded strings.Builder
	encoded.WriteByte('[')
	for index, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return "", fmt.Errorf("embedding contains a non-finite value")
		}
		if index > 0 {
			encoded.WriteByte(',')
		}
		encoded.WriteString(strconv.FormatFloat(value, 'g', -1, 64))
	}
	encoded.WriteByte(']')
	return encoded.String(), nil
}

func (database *postgresTransaction) PutSourceVector(identifier string, position int, model string, categories []string, chunk vectorChunk, deleted bool) error {
	vector, operationError := encodeVector(chunk.Vector)
	if operationError != nil {
		return operationError
	}
	if categories == nil {
		categories = []string{}
	}
	_, operationError = database.transaction.Exec(database.operationContext, `INSERT INTO context_plugin.vectors(workspace_id,kind,key,version,chunk,model,categories,content,embedding,deleted,historical) VALUES($1,'source',$2,1,$3,$4,$5,$6,$7::vector,$8,false) ON CONFLICT(workspace_id,kind,key,version,chunk) DO UPDATE SET model=EXCLUDED.model,categories=EXCLUDED.categories,content=EXCLUDED.content,embedding=EXCLUDED.embedding,deleted=EXCLUDED.deleted`, database.workspaceID, identifier, position, model, categories, chunk.Text, vector, deleted)
	return operationError
}
