package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type secrets struct{ store *Store }

func (secretStore *secrets) SaveProviderKey(operationContext context.Context, provider string, key string) error {
	_, operationError := secretStore.store.pool.Exec(operationContext, `
		INSERT INTO provider_keys (scope, provider, key) VALUES ('', $1, $2)
		ON CONFLICT (scope, provider) DO UPDATE SET key = EXCLUDED.key, updated_at = now()`, provider, key)
	return operationError
}

func (secretStore *secrets) ProviderKey(operationContext context.Context, provider string) (string, error) {
	var key string
	operationError := secretStore.store.pool.QueryRow(operationContext, `SELECT key FROM provider_keys WHERE scope = '' AND provider = $1`, provider).Scan(&key)
	if operationError == pgx.ErrNoRows {
		return "", nil
	}
	return key, operationError
}

func (secretStore *secrets) SaveInstanceKey(operationContext context.Context, instanceID string, provider string, key string) error {
	_, operationError := secretStore.store.pool.Exec(operationContext, `
		INSERT INTO provider_keys (scope, provider, key) VALUES ($1, $2, $3)
		ON CONFLICT (scope, provider) DO UPDATE SET key = EXCLUDED.key, updated_at = now()`, instanceID, provider, key)
	return operationError
}

func (secretStore *secrets) InstanceKey(operationContext context.Context, instanceID string, provider string) (string, error) {
	var key string
	operationError := secretStore.store.pool.QueryRow(operationContext, `SELECT key FROM provider_keys WHERE scope = $1 AND provider = $2`, instanceID, provider).Scan(&key)
	if operationError == pgx.ErrNoRows {
		return "", nil
	}
	return key, operationError
}

func (secretStore *secrets) ResolveKey(operationContext context.Context, instanceID string, provider string) (string, error) {
	if instanceID != "" {
		value, operationError := secretStore.InstanceKey(operationContext, instanceID, provider)
		if operationError != nil {
			return "", operationError
		}
		if value != "" {
			return value, nil
		}
	}
	return secretStore.ProviderKey(operationContext, provider)
}

func (secretStore *secrets) DeleteProviderKey(operationContext context.Context, provider string) error {
	_, operationError := secretStore.store.pool.Exec(operationContext, `DELETE FROM provider_keys WHERE scope = '' AND provider = $1`, provider)
	return operationError
}

func (secretStore *secrets) DeleteInstanceKey(operationContext context.Context, instanceID string, provider string) error {
	_, operationError := secretStore.store.pool.Exec(operationContext, `DELETE FROM provider_keys WHERE scope = $1 AND provider = $2`, instanceID, provider)
	return operationError
}
