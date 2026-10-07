package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

const schema = `
CREATE TABLE IF NOT EXISTS instances (
	id text PRIMARY KEY,
	workspace text NOT NULL,
	models jsonb NOT NULL DEFAULT '[]',
	default_model text NOT NULL DEFAULT '',
	process_limit int NOT NULL DEFAULT 0,
	agent_depth_limit int NOT NULL DEFAULT 0,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS sessions (
	id text PRIMARY KEY,
	instance_id text NOT NULL,
	parent_id text NOT NULL DEFAULT '',
	depth int NOT NULL DEFAULT 0,
	model text NOT NULL DEFAULT '',
	created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE instances ADD COLUMN IF NOT EXISTS stopped boolean NOT NULL DEFAULT false;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS completed boolean NOT NULL DEFAULT false;
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS reasoning_effort text NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS deleted boolean NOT NULL DEFAULT false;
CREATE TABLE IF NOT EXISTS session_task_state (
	session_id text PRIMARY KEY REFERENCES sessions(id),
	todo jsonb NOT NULL DEFAULT '[]',
	doing jsonb,
	revision bigint NOT NULL DEFAULT 0,
	responses_since_update int NOT NULL DEFAULT 0,
	updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS messages (
	id text PRIMARY KEY,
	session_id text NOT NULL,
	seq bigserial,
	role text NOT NULL,
	content jsonb NOT NULL,
	tool_calls jsonb NOT NULL DEFAULT '[]',
	tool_call_id text NOT NULL DEFAULT '',
	usage jsonb,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS message_queue (
	id text PRIMARY KEY,
	session_id text NOT NULL,
	seq bigserial,
	message jsonb NOT NULL,
	running boolean NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS message_queue_session ON message_queue (session_id, seq);
ALTER TABLE messages ADD COLUMN IF NOT EXISTS provider_state jsonb;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS reasoning text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS events (
	seq bigserial PRIMARY KEY,
	instance_id text NOT NULL,
	session_id text NOT NULL,
	name text NOT NULL,
	payload jsonb,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS processes (
	id text PRIMARY KEY,
	instance_id text NOT NULL,
	session_id text NOT NULL,
	spec jsonb NOT NULL,
	pid int NOT NULL DEFAULT 0,
	status text NOT NULL DEFAULT '',
	exit jsonb,
	started_at timestamptz,
	ended_at timestamptz
);
CREATE TABLE IF NOT EXISTS permissions (
	request_id text PRIMARY KEY,
	kind text NOT NULL,
	scope text NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS usage_records (
	id bigserial PRIMARY KEY,
	instance_id text NOT NULL,
	session_id text NOT NULL,
	model_id text NOT NULL,
	input_tokens int NOT NULL DEFAULT 0,
	cache_read_tokens int NOT NULL DEFAULT 0,
	cache_write_tokens int NOT NULL DEFAULT 0,
	output_tokens int NOT NULL DEFAULT 0,
	reasoning_tokens int NOT NULL DEFAULT 0,
	cost_currency text NOT NULL DEFAULT '',
	cost_value double precision NOT NULL DEFAULT 0,
	created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE permissions ADD COLUMN IF NOT EXISTS instance_id text NOT NULL DEFAULT '';
-- Earlier harness versions calculated every recorded cost from catalog rates.
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS cost_estimated boolean;
UPDATE usage_records SET cost_estimated = cost_currency <> '' WHERE cost_estimated IS NULL;
ALTER TABLE usage_records ALTER COLUMN cost_estimated SET DEFAULT false;
ALTER TABLE usage_records ALTER COLUMN cost_estimated SET NOT NULL;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS request_id text;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS agent text NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS run_id text NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS source_session_id text NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS duration_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS usage_request_once ON usage_records (instance_id, request_id) WHERE request_id IS NOT NULL;
ALTER TABLE permissions ADD COLUMN IF NOT EXISTS session_id text NOT NULL DEFAULT '';
ALTER TABLE permissions ADD COLUMN IF NOT EXISTS target text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS providers (
	name text PRIMARY KEY,
	api_url text NOT NULL DEFAULT '',
	model_list_url text NOT NULL DEFAULT '',
	price_table_url text NOT NULL DEFAULT '',
	interval_seconds bigint NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS provider_keys (
	scope text NOT NULL,
	provider text NOT NULL,
	key text NOT NULL,
	updated_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (scope, provider)
);
CREATE TABLE IF NOT EXISTS settings (
	scope text NOT NULL,
	key text NOT NULL,
	value text NOT NULL,
	updated_at timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (scope, key)
);
CREATE TABLE IF NOT EXISTS models (
	provider text NOT NULL,
	id text NOT NULL,
	level int NOT NULL DEFAULT 0,
	input jsonb NOT NULL DEFAULT '[]',
	output jsonb NOT NULL DEFAULT '[]',
	tools boolean NOT NULL DEFAULT false,
	context_max int NOT NULL DEFAULT 0,
	prices jsonb,
	PRIMARY KEY (provider, id)
);
ALTER TABLE models ADD COLUMN IF NOT EXISTS name text NOT NULL DEFAULT '';
ALTER TABLE models ADD COLUMN IF NOT EXISTS reasoning boolean NOT NULL DEFAULT false;
ALTER TABLE models ADD COLUMN IF NOT EXISTS reasoning_efforts jsonb NOT NULL DEFAULT '[]';
ALTER TABLE models ADD COLUMN IF NOT EXISTS default_reasoning_effort text NOT NULL DEFAULT '';
ALTER TABLE models ADD COLUMN IF NOT EXISTS reasoning_summary text NOT NULL DEFAULT '';
ALTER TABLE models ADD COLUMN IF NOT EXISTS billing text NOT NULL DEFAULT '';
-- Legacy false values could mean missing capability metadata. Preserve explicit
-- false values written after this one-time nullable-column migration.
ALTER TABLE models ADD COLUMN IF NOT EXISTS tool_support_unknown boolean;
UPDATE models SET tool_support_unknown = NOT tools WHERE tool_support_unknown IS NULL;
ALTER TABLE models ALTER COLUMN tool_support_unknown SET DEFAULT false;
ALTER TABLE models ALTER COLUMN tool_support_unknown SET NOT NULL;
ALTER TABLE providers ADD COLUMN IF NOT EXISTS protocol text NOT NULL DEFAULT '';
ALTER TABLE providers ADD COLUMN IF NOT EXISTS authentication text NOT NULL DEFAULT '';
ALTER TABLE providers ADD COLUMN IF NOT EXISTS model_list_format text NOT NULL DEFAULT '';
ALTER TABLE providers ADD COLUMN IF NOT EXISTS metadata_url text NOT NULL DEFAULT '';
ALTER TABLE providers ADD COLUMN IF NOT EXISTS metadata_format text NOT NULL DEFAULT '';
ALTER TABLE providers ADD COLUMN IF NOT EXISTS metadata_provider text NOT NULL DEFAULT '';
ALTER TABLE providers ADD COLUMN IF NOT EXISTS billing text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS provider_oauth (
	provider text PRIMARY KEY,
	access_token text NOT NULL,
	refresh_token text NOT NULL,
	account_id text NOT NULL DEFAULT '',
	residency text NOT NULL DEFAULT '',
	expires_at timestamptz NOT NULL
);
`

type Store struct {
	pool *pgxpool.Pool
}

func Open(operationContext context.Context, databaseURL string) (*Store, error) {
	configuration, operationError := pgxpool.ParseConfig(databaseURL)
	if operationError != nil {
		return nil, operationError
	}
	pool, operationError := pgxpool.NewWithConfig(operationContext, configuration)
	if operationError != nil {
		return nil, operationError
	}
	if operationError := pool.Ping(operationContext); operationError != nil {
		pool.Close()
		return nil, operationError
	}
	if operationError := initializeSchema(operationContext, pool); operationError != nil {
		pool.Close()
		return nil, operationError
	}
	return &Store{pool: pool}, nil
}

// Concurrent application starts and test packages share the same catalog.
// IF NOT EXISTS alone does not serialize PostgreSQL type creation.
func initializeSchema(operationContext context.Context, pool *pgxpool.Pool) error {
	transaction, operationError := pool.Begin(operationContext)
	if operationError != nil {
		return operationError
	}
	defer transaction.Rollback(context.WithoutCancel(operationContext))
	if _, operationError := transaction.Exec(operationContext, `SELECT pg_advisory_xact_lock(71393000)`); operationError != nil {
		return operationError
	}
	if _, operationError := transaction.Exec(operationContext, schema); operationError != nil {
		return operationError
	}
	return transaction.Commit(operationContext)
}

func (database *Store) Close() error {
	database.pool.Close()
	return nil
}

func (database *Store) Instances() store.InstanceStore {
	return &instances{database}
}
func (database *Store) Sessions() store.SessionStore {
	return &sessions{database}
}
func (database *Store) TaskStates() store.TaskStateStore { return &taskStates{database} }
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

func (database *Store) Queue() store.QueueStore {
	return &queueStore{store: database}
}
