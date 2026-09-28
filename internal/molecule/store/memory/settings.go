package memory

import (
	"context"
	"strings"
)

func settingOf(scope string, key string) string {
	return scope + "/" + key
}

type settings struct{ store *Store }

func (settingsStore *settings) Save(operationContext context.Context, scope string, key string, value string) error {
	settingsStore.store.mutex.Lock()
	defer settingsStore.store.mutex.Unlock()
	settingsStore.store.settings[settingOf(scope, key)] = value
	return nil
}

func (settingsStore *settings) Get(operationContext context.Context, scope string, key string) (string, error) {
	settingsStore.store.mutex.RLock()
	defer settingsStore.store.mutex.RUnlock()
	return settingsStore.store.settings[settingOf(scope, key)], nil
}

func (settingsStore *settings) All(operationContext context.Context, scope string) (map[string]string, error) {
	settingsStore.store.mutex.RLock()
	defer settingsStore.store.mutex.RUnlock()
	result := map[string]string{}
	prefix := scope + "/"
	for compound, value := range settingsStore.store.settings {
		if strings.HasPrefix(compound, prefix) {
			result[strings.TrimPrefix(compound, prefix)] = value
		}
	}
	return result, nil
}

func (settingsStore *settings) Delete(operationContext context.Context, scope string, key string) error {
	settingsStore.store.mutex.Lock()
	defer settingsStore.store.mutex.Unlock()
	delete(settingsStore.store.settings, settingOf(scope, key))
	return nil
}

func (settingsStore *settings) Resolve(operationContext context.Context, instanceID string, key string) (string, error) {
	settingsStore.store.mutex.RLock()
	defer settingsStore.store.mutex.RUnlock()
	if value := settingsStore.store.settings[settingOf(instanceID, key)]; value != "" {
		return value, nil
	}
	return settingsStore.store.settings[settingOf("", key)], nil
}
