package store

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type InstanceStore interface {
	Save(ctx context.Context, spec atom.InstanceSpec) error
	Get(ctx context.Context, id string) (atom.InstanceSpec, error)
	All(ctx context.Context) ([]atom.InstanceSpec, error)
	Delete(ctx context.Context, id string) error
}

type SessionStore interface {
	Save(ctx context.Context, session atom.Session) error
	Get(ctx context.Context, id atom.SessionID) (atom.Session, error)
	Agents(ctx context.Context, parent atom.SessionID) ([]atom.SessionID, error)
	Append(ctx context.Context, message atom.Message) error
	Messages(ctx context.Context, id atom.SessionID) ([]atom.Message, error)
}

type EventStore interface {
	Append(ctx context.Context, event atom.Event) error
	Since(ctx context.Context, instanceID string, seq uint64) ([]atom.Event, error)
}

type ProcessStore interface {
	Save(ctx context.Context, process atom.ProcessRecord) error
	Get(ctx context.Context, id string) (atom.ProcessRecord, error)
	List(ctx context.Context, session atom.SessionID) ([]atom.ProcessRecord, error)
}

type PermissionStore interface {
	Save(ctx context.Context, decision atom.PermissionDecision) error
	Get(ctx context.Context, id string) (atom.PermissionDecision, error)
}

type UsageStore interface {
	Save(ctx context.Context, record atom.UsageRecord) error
	Session(ctx context.Context, id atom.SessionID) (atom.Statistics, error)
	Instance(ctx context.Context, id string) (atom.Statistics, error)
	All(ctx context.Context) (atom.Statistics, error)
}

type ProviderStore interface {
	Save(ctx context.Context, spec atom.ProviderSpec) error
	Get(ctx context.Context, id string) (atom.ProviderSpec, error)
	All(ctx context.Context) ([]atom.ProviderSpec, error)
	Delete(ctx context.Context, id string) error
	SaveModels(ctx context.Context, provider string, models []atom.ModelInfo) error
	Models(ctx context.Context, provider string) ([]atom.ModelInfo, error)
}

type Store interface {
	Instances() InstanceStore
	Sessions() SessionStore
	Events() EventStore
	Processes() ProcessStore
	Permissions() PermissionStore
	Usage() UsageStore
	Providers() ProviderStore
	Close() error
}
