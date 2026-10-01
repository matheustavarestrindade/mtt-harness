package memory

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func providerSecretKey(scope string, provider string) string {
	return scope + "/" + provider
}

type secrets struct{ store *Store }

func (secretStore *secrets) SaveProviderKey(operationContext context.Context, provider string, key string) error {
	secretStore.store.mutex.Lock()
	defer secretStore.store.mutex.Unlock()
	secretStore.store.secretKeys[providerSecretKey("", provider)] = atom.ProviderKey{Scope: "", Provider: provider, Key: key}
	return nil
}

func (secretStore *secrets) ProviderKey(operationContext context.Context, provider string) (string, error) {
	secretStore.store.mutex.RLock()
	defer secretStore.store.mutex.RUnlock()
	return secretStore.store.secretKeys[providerSecretKey("", provider)].Key, nil
}

func (secretStore *secrets) SaveInstanceKey(operationContext context.Context, instanceID string, provider string, key string) error {
	secretStore.store.mutex.Lock()
	defer secretStore.store.mutex.Unlock()
	secretStore.store.secretKeys[providerSecretKey(instanceID, provider)] = atom.ProviderKey{Scope: instanceID, Provider: provider, Key: key}
	return nil
}

func (secretStore *secrets) InstanceKey(operationContext context.Context, instanceID string, provider string) (string, error) {
	secretStore.store.mutex.RLock()
	defer secretStore.store.mutex.RUnlock()
	return secretStore.store.secretKeys[providerSecretKey(instanceID, provider)].Key, nil
}

func (secretStore *secrets) ResolveKey(operationContext context.Context, instanceID string, provider string) (string, error) {
	secretStore.store.mutex.RLock()
	defer secretStore.store.mutex.RUnlock()
	if key := secretStore.store.secretKeys[providerSecretKey(instanceID, provider)].Key; key != "" {
		return key, nil
	}
	return secretStore.store.secretKeys[providerSecretKey("", provider)].Key, nil
}

func (secretStore *secrets) DeleteProviderKey(operationContext context.Context, provider string) error {
	secretStore.store.mutex.Lock()
	defer secretStore.store.mutex.Unlock()
	delete(secretStore.store.secretKeys, providerSecretKey("", provider))
	return nil
}

func (secretStore *secrets) DeleteInstanceKey(operationContext context.Context, instanceID string, provider string) error {
	secretStore.store.mutex.Lock()
	defer secretStore.store.mutex.Unlock()
	delete(secretStore.store.secretKeys, providerSecretKey(instanceID, provider))
	return nil
}
