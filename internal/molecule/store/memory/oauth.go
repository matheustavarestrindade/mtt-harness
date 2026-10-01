package memory

import (
	"context"
	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (secretStore *secrets) SaveOAuthCredential(operationContext context.Context, provider string, credential atom.OAuthCredential) error {
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	secretStore.store.mutex.Lock()
	defer secretStore.store.mutex.Unlock()
	secretStore.store.oauthCredentials[provider] = credential
	return nil
}

func (secretStore *secrets) OAuthCredential(operationContext context.Context, provider string) (atom.OAuthCredential, error) {
	secretStore.store.mutex.RLock()
	defer secretStore.store.mutex.RUnlock()
	return secretStore.store.oauthCredentials[provider], operationContext.Err()
}

func (secretStore *secrets) DeleteOAuthCredential(operationContext context.Context, provider string) error {
	if operationError := operationContext.Err(); operationError != nil {
		return operationError
	}
	secretStore.store.mutex.Lock()
	defer secretStore.store.mutex.Unlock()
	delete(secretStore.store.oauthCredentials, provider)
	return nil
}
