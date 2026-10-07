package contextplugin

import (
	"context"
	"encoding/json"
)

// repository keeps inference outside transactions. Every operation is scoped
// to one workspace; a transaction owns only short state and vector updates.
type repository interface {
	Read(context.Context, string, string, string) (json.RawMessage, error)
	List(context.Context, string, string, string, int) ([]json.RawMessage, error)
	Transact(context.Context, string, func(transaction) error) error
	PendingWorkspaces(context.Context, int) ([]string, error)
	Search(context.Context, string, searchQuery) ([]searchHit, error)
	Count(context.Context, string) (map[string]int64, error)
	Metric(context.Context, string, string, string, int64, int64) error
	Close()
}

type transaction interface {
	Get(string, string) (json.RawMessage, error)
	List(string) ([]json.RawMessage, error)
	Put(string, string, any) error
	NextSourceID() (int64, error)
	ReplaceVectors(string, string, int, string, []string, []vectorChunk, bool, bool) error
	MarkVectors(string, string, bool, bool) error
	PutSourceVector(string, int, string, []string, vectorChunk, bool) error
	AddMetric(string, string, int64, int64) error
}

func readValue[Value any](operationContext context.Context, database repository, workspaceID, kind, identifier string) (Value, error) {
	var value Value
	data, operationError := database.Read(operationContext, workspaceID, kind, identifier)
	if operationError != nil || len(data) == 0 {
		return value, operationError
	}
	operationError = json.Unmarshal(data, &value)
	return value, operationError
}

func readTransactionValue[Value any](database transaction, kind, identifier string) (Value, error) {
	var value Value
	data, operationError := database.Get(kind, identifier)
	if operationError != nil || len(data) == 0 {
		return value, operationError
	}
	operationError = json.Unmarshal(data, &value)
	return value, operationError
}

func readTransactionValues[Value any](database transaction, kind string) ([]Value, error) {
	data, operationError := database.List(kind)
	if operationError != nil {
		return nil, operationError
	}
	values := make([]Value, 0, len(data))
	for _, document := range data {
		var value Value
		if operationError := json.Unmarshal(document, &value); operationError != nil {
			return nil, operationError
		}
		values = append(values, value)
	}
	return values, nil
}
