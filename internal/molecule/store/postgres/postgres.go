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
-- Legacy false values could mean missing capability metadata. Preserve explicit
-- false values written after this one-time nullable-column migration.
ALTER TABLE models ADD COLUMN IF NOT EXISTS tool_support_unknown boolean;
UPDATE models SET tool_support_unknown = NOT tools WHERE tool_support_unknown IS NULL;
ALTER TABLE models ALTER COLUMN tool_support_unknown SET DEFAULT false;
ALTER TABLE models ALTER COLUMN tool_support_unknown SET NOT NULL;
ALTER TABLE providers ADD COLUMN IF NOT EXISTS protocol text NOT NULL DEFAULT '';
ALTER TABLE providers ADD COLUMN IF NOT EXISTS authentication text NOT NULL DEFAULT '';
ALTER TABLE providers ADD COLUMN IF NOT EXISTS model_list_format text NOT NULL DEFAULT '';
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
	if _, operationError := pool.Exec(operationContext, schema); operationError != nil {
		pool.Close()
		return nil, operationError
	}
	return &Store{pool: pool}, nil
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
