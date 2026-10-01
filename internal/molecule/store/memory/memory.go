package memory

import (
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type Store struct {
	mutex            sync.RWMutex
	instances        map[string]atom.InstanceSpec
	sessions         map[atom.SessionID]atom.Session
	messages         map[atom.SessionID][]atom.Message
	events           []atom.Event
	sequenceNumber   uint64
	processes        map[string]atom.ProcessRecord
	permissions      map[string]atom.PermissionDecision
	usage            []atom.UsageRecord
	providers        map[string]atom.ProviderSpec
	models           map[string][]atom.ModelInfo
	secretKeys       map[string]atom.ProviderKey
	oauthCredentials map[string]atom.OAuthCredential
	settings         map[string]string
	queued           map[string]atom.QueuedMessage
	queueSequence    uint64
}

func New() *Store {
	return &Store{
		instances:        map[string]atom.InstanceSpec{},
		sessions:         map[atom.SessionID]atom.Session{},
		messages:         map[atom.SessionID][]atom.Message{},
		processes:        map[string]atom.ProcessRecord{},
		permissions:      map[string]atom.PermissionDecision{},
		providers:        map[string]atom.ProviderSpec{},
		models:           map[string][]atom.ModelInfo{},
		secretKeys:       map[string]atom.ProviderKey{},
		oauthCredentials: map[string]atom.OAuthCredential{},
		settings:         map[string]string{},
		queued:           map[string]atom.QueuedMessage{},
	}
}

func (database *Store) Instances() store.InstanceStore {
	return &instances{database}
}
func (database *Store) Sessions() store.SessionStore {
	return &sessions{database}
}
func (database *Store) Events() store.EventStore {
	return &events{database}
}
func (database *Store) Processes() store.ProcessStore {
	return &processes{database}
}
func (database *Store) Permissions() store.PermissionStore {
	return &permissions{database}
}
func (database *Store) Usage() store.UsageStore {
	return &usage{database}
}
func (database *Store) Providers() store.ProviderStore {
	return &providers{database}
}
func (database *Store) Secrets() store.SecretStore {
	return &secrets{database}
}
func (database *Store) Settings() store.SettingsStore {
	return &settings{database}
}
func (database *Store) Close() error {
	return nil
}

func (database *Store) Queue() store.QueueStore {
	return &queueStore{store: database}
}
