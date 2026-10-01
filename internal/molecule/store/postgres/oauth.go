package postgres

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (secretStore *secrets) SaveOAuthCredential(operationContext context.Context, provider string, credential atom.OAuthCredential) error {
	_, operationError := secretStore.store.pool.Exec(operationContext, `
		INSERT INTO provider_oauth (provider, access_token, refresh_token, account_id, residency, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (provider) DO UPDATE SET access_token=EXCLUDED.access_token,
		refresh_token=EXCLUDED.refresh_token, account_id=EXCLUDED.account_id,
		residency=EXCLUDED.residency, expires_at=EXCLUDED.expires_at`,
		provider, credential.AccessToken, credential.RefreshToken, credential.AccountID, credential.Residency, credential.ExpiresAt)
	return operationError
}

func (secretStore *secrets) OAuthCredential(operationContext context.Context, provider string) (atom.OAuthCredential, error) {
	var credential atom.OAuthCredential
	operationError := secretStore.store.pool.QueryRow(operationContext, `
		SELECT access_token, refresh_token, account_id, residency, expires_at FROM provider_oauth WHERE provider=$1`, provider).
		Scan(&credential.AccessToken, &credential.RefreshToken, &credential.AccountID, &credential.Residency, &credential.ExpiresAt)
	if errors.Is(operationError, pgx.ErrNoRows) {
		return atom.OAuthCredential{}, nil
	}
	return credential, operationError
}

func (secretStore *secrets) DeleteOAuthCredential(operationContext context.Context, provider string) error {
	_, operationError := secretStore.store.pool.Exec(operationContext, `DELETE FROM provider_oauth WHERE provider=$1`, provider)
	return operationError
}
