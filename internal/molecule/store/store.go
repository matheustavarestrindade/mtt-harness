package store

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type InstanceStore interface {
	Save(operationContext context.Context, instanceSpec atom.InstanceSpec) error
	Get(operationContext context.Context, identifier string) (atom.InstanceSpec, error)
	All(operationContext context.Context) ([]atom.InstanceSpec, error)
	Delete(operationContext context.Context, identifier string) error
}

// ErrSessionSelectionChanged means the caller validated a stale selection or
// the session completed before the change. Read current state before retrying.
var ErrSessionSelectionChanged = errors.New("session settings changed; read the session and retry")

var ErrSessionDeleted = errors.New("session has been deleted")
var ErrSessionNotFound = errors.New("session is not in the store")
var ErrConversationBusy = errors.New("session or child session has active or queued work; stop it before deleting")

type SessionStore interface {
	// Save initializes a new session's model selection. Lifecycle saves retain
	// the stored selection; SetModelSelection owns subsequent changes.
	Save(operationContext context.Context, session atom.Session) error
	SetModelSelection(operationContext context.Context, sessionID atom.SessionID, previous, next atom.SessionModelSelection) error
	// GetModelSelection distinguishes an absent record from a stored default.
	// Caller-owned sessions without a record retain their supplied selection.
	GetModelSelection(operationContext context.Context, sessionID atom.SessionID) (atom.SessionModelSelection, bool, error)
	Get(operationContext context.Context, sessionID atom.SessionID) (atom.Session, error)
	Agents(operationContext context.Context, parent atom.SessionID) ([]atom.SessionID, error)
	List(operationContext context.Context, instanceID string) ([]atom.Session, error)
	Append(operationContext context.Context, message atom.Message) error
	Messages(operationContext context.Context, sessionID atom.SessionID) ([]atom.Message, error)
	DeleteAfter(operationContext context.Context, sessionID atom.SessionID, messageID string) (int, error)
	// DeleteConversation removes saved conversation data for a session tree.
	// ID/parent tombstones and usage records remain; stale writes must fail.
	DeleteConversation(operationContext context.Context, sessionID atom.SessionID) ([]atom.SessionID, error)
}

type QueueStore interface {
	Enqueue(operationContext context.Context, message atom.Message, limit int) error
	All(operationContext context.Context) ([]atom.QueuedMessage, error)
	Start(operationContext context.Context, messageID string) error
	Finish(operationContext context.Context, messageID string) error
	Remove(operationContext context.Context, sessionID atom.SessionID, messageID string) (bool, error)
	ClearPending(operationContext context.Context, sessionID atom.SessionID) (int, error)
}

type EventStore interface {
	Append(operationContext context.Context, event atom.Event) error
	Record(operationContext context.Context, event atom.Event) (atom.Event, error)
	Since(operationContext context.Context, instanceID string, sequenceNumber uint64) ([]atom.Event, error)
}

type ProcessStore interface {
	Save(operationContext context.Context, process atom.ProcessRecord) error
	Get(operationContext context.Context, identifier string) (atom.ProcessRecord, error)
	List(operationContext context.Context, session atom.SessionID) ([]atom.ProcessRecord, error)
	CountRunning(operationContext context.Context, instanceID string) (int, error)
}

type PermissionStore interface {
	Save(operationContext context.Context, decision atom.PermissionDecision) error
	Get(operationContext context.Context, identifier string) (atom.PermissionDecision, error)
	Resolve(operationContext context.Context, session atom.Session, target string) (atom.PermissionDecision, bool, error)
}

type UsageStore interface {
	Save(operationContext context.Context, record atom.UsageRecord) error
	Session(operationContext context.Context, sessionID atom.SessionID) (atom.Statistics, error)
	Instance(operationContext context.Context, identifier string) (atom.Statistics, error)
	All(operationContext context.Context) (atom.Statistics, error)
	Agents(operationContext context.Context, instanceID string) ([]atom.AgentStatistics, error)
}

type ProviderStore interface {
	Save(operationContext context.Context, providerSpec atom.ProviderSpec) error
	Get(operationContext context.Context, identifier string) (atom.ProviderSpec, error)
	All(operationContext context.Context) ([]atom.ProviderSpec, error)
	Delete(operationContext context.Context, identifier string) error
	SaveModels(operationContext context.Context, provider string, models []atom.ModelInfo) error
	Models(operationContext context.Context, provider string) ([]atom.ModelInfo, error)
}

type SecretStore interface {
	SaveOAuthCredential(operationContext context.Context, provider string, credential atom.OAuthCredential) error
	OAuthCredential(operationContext context.Context, provider string) (atom.OAuthCredential, error)
	DeleteOAuthCredential(operationContext context.Context, provider string) error
	SaveProviderKey(operationContext context.Context, provider string, key string) error
	ProviderKey(operationContext context.Context, provider string) (string, error)
	SaveInstanceKey(operationContext context.Context, instanceID string, provider string, key string) error
	InstanceKey(operationContext context.Context, instanceID string, provider string) (string, error)
	ResolveKey(operationContext context.Context, instanceID string, provider string) (string, error)
	DeleteProviderKey(operationContext context.Context, provider string) error
	DeleteInstanceKey(operationContext context.Context, instanceID string, provider string) error
}

type SettingsStore interface {
	Save(operationContext context.Context, scope string, key string, value string) error
	Get(operationContext context.Context, scope string, key string) (string, error)
	All(operationContext context.Context, scope string) (map[string]string, error)
	Delete(operationContext context.Context, scope string, key string) error
	Resolve(operationContext context.Context, instanceID string, key string) (string, error)
}

type Store interface {
	Queue() QueueStore
	Instances() InstanceStore
	Sessions() SessionStore
	TaskStates() TaskStateStore
	Events() EventStore
	Processes() ProcessStore
	Permissions() PermissionStore
	Usage() UsageStore
	Providers() ProviderStore
	Secrets() SecretStore
	Settings() SettingsStore
	Close() error
}
