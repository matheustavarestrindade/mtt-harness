package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type settings struct{ store *Store }

func (settingsStore *settings) Save(operationContext context.Context, scope string, key string, value string) error {
	_, operationError := settingsStore.store.pool.Exec(operationContext, `
		INSERT INTO settings (scope, key, value) VALUES ($1, $2, $3)
		ON CONFLICT (scope, key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, scope, key, value)
	return operationError
}

func (settingsStore *settings) Get(operationContext context.Context, scope string, key string) (string, error) {
	var value string
	operationError := settingsStore.store.pool.QueryRow(operationContext, `SELECT value FROM settings WHERE scope = $1 AND key = $2`, scope, key).Scan(&value)
	if operationError == pgx.ErrNoRows {
		return "", nil
	}
	return value, operationError
}

func (settingsStore *settings) All(operationContext context.Context, scope string) (map[string]string, error) {
	rows, operationError := settingsStore.store.pool.Query(operationContext, `SELECT key, value FROM settings WHERE scope = $1 ORDER BY key`, scope)
	if operationError != nil {
		return nil, operationError
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var key, value string
		if operationError := rows.Scan(&key, &value); operationError != nil {
			return nil, operationError
		}
		result[key] = value
	}
	return result, rows.Err()
}

func (settingsStore *settings) Delete(operationContext context.Context, scope string, key string) error {
	_, operationError := settingsStore.store.pool.Exec(operationContext, `DELETE FROM settings WHERE scope = $1 AND key = $2`, scope, key)
	return operationError
}

func (settingsStore *settings) Resolve(operationContext context.Context, instanceID string, key string) (string, error) {
	if instanceID != "" {
		value, operationError := settingsStore.Get(operationContext, instanceID, key)
		if operationError != nil {
			return "", operationError
		}
		if value != "" {
			return value, nil
		}
	}
	return settingsStore.Get(operationContext, "", key)
}
