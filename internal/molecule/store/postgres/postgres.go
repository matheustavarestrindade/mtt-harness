package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/matheustavarestrindade/mtt-harness/atom"
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
`

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() error {
	s.pool.Close()
	return nil
}

func (s *Store) Instances() store.InstanceStore     { return &instances{s} }
func (s *Store) Sessions() store.SessionStore       { return &sessions{s} }
func (s *Store) Events() store.EventStore           { return &events{s} }
func (s *Store) Processes() store.ProcessStore      { return &processes{s} }
func (s *Store) Permissions() store.PermissionStore { return &permissions{s} }
func (s *Store) Usage() store.UsageStore            { return &usage{s} }
func (s *Store) Providers() store.ProviderStore     { return &providers{s} }
func (s *Store) Secrets() store.SecretStore         { return &secrets{s} }
func (s *Store) Settings() store.SettingsStore      { return &settings{s} }

type instances struct{ s *Store }

func (i *instances) Save(ctx context.Context, spec atom.InstanceSpec) error {
	models, _ := json.Marshal(spec.Models)
	_, err := i.s.pool.Exec(ctx, `
		INSERT INTO instances (id, workspace, models, default_model, process_limit, agent_depth_limit, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			workspace = EXCLUDED.workspace,
			models = EXCLUDED.models,
			default_model = EXCLUDED.default_model,
			process_limit = EXCLUDED.process_limit,
			agent_depth_limit = EXCLUDED.agent_depth_limit`,
		spec.ID, spec.Workspace, models, spec.DefaultModel, spec.ProcessLimit, spec.AgentDepthLimit, spec.CreatedAt)
	return err
}

func (i *instances) Get(ctx context.Context, id string) (atom.InstanceSpec, error) {
	var spec atom.InstanceSpec
	var models []byte
	err := i.s.pool.QueryRow(ctx, `
		SELECT id, workspace, models, default_model, process_limit, agent_depth_limit, created_at
		FROM instances WHERE id = $1`, id).
		Scan(&spec.ID, &spec.Workspace, &models, &spec.DefaultModel, &spec.ProcessLimit, &spec.AgentDepthLimit, &spec.CreatedAt)
	if err != nil {
		return atom.InstanceSpec{}, err
	}
	_ = json.Unmarshal(models, &spec.Models)
	return spec, nil
}

func (i *instances) All(ctx context.Context) ([]atom.InstanceSpec, error) {
	rows, err := i.s.pool.Query(ctx, `
		SELECT id, workspace, models, default_model, process_limit, agent_depth_limit, created_at
		FROM instances ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []atom.InstanceSpec
	for rows.Next() {
		var spec atom.InstanceSpec
		var models []byte
		if err := rows.Scan(&spec.ID, &spec.Workspace, &models, &spec.DefaultModel, &spec.ProcessLimit, &spec.AgentDepthLimit, &spec.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(models, &spec.Models)
		list = append(list, spec)
	}
	return list, rows.Err()
}

func (i *instances) Delete(ctx context.Context, id string) error {
	_, err := i.s.pool.Exec(ctx, `DELETE FROM instances WHERE id = $1`, id)
	return err
}

type sessions struct{ s *Store }

func (s *sessions) Save(ctx context.Context, session atom.Session) error {
	_, err := s.s.pool.Exec(ctx, `
		INSERT INTO sessions (id, instance_id, parent_id, depth, model, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			parent_id = EXCLUDED.parent_id,
			depth = EXCLUDED.depth,
			model = EXCLUDED.model`,
		string(session.ID), session.InstanceID, string(session.Parent), session.Depth, session.Model, session.CreatedAt)
	return err
}

func (s *sessions) Get(ctx context.Context, id atom.SessionID) (atom.Session, error) {
	var session atom.Session
	err := s.s.pool.QueryRow(ctx, `
		SELECT id, instance_id, parent_id, depth, model, created_at
		FROM sessions WHERE id = $1`, string(id)).
		Scan(&session.ID, &session.InstanceID, &session.Parent, &session.Depth, &session.Model, &session.CreatedAt)
	return session, err
}

func (s *sessions) Agents(ctx context.Context, parent atom.SessionID) ([]atom.SessionID, error) {
	var rows pgx.Rows
	var err error
	if parent == "" {
		rows, err = s.s.pool.Query(ctx, `SELECT id FROM sessions WHERE parent_id = '' ORDER BY id`)
	} else {
		rows, err = s.s.pool.Query(ctx, `SELECT id FROM sessions WHERE parent_id = $1 ORDER BY id`, string(parent))
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []atom.SessionID
	for rows.Next() {
		var id atom.SessionID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		list = append(list, id)
	}
	return list, rows.Err()
}

func (s *sessions) Append(ctx context.Context, message atom.Message) error {
	content, _ := json.Marshal(message.Content)
	calls, _ := json.Marshal(message.ToolCalls)
	var usage any
	if message.Usage != nil {
		data, _ := json.Marshal(message.Usage)
		usage = data
	}
	_, err := s.s.pool.Exec(ctx, `
		INSERT INTO messages (id, session_id, role, content, tool_calls, tool_call_id, usage, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		message.ID, string(message.SessionID), string(message.Role), content, calls, message.ToolCallID, usage, message.CreatedAt)
	return err
}

func (s *sessions) Messages(ctx context.Context, id atom.SessionID) ([]atom.Message, error) {
	rows, err := s.s.pool.Query(ctx, `
		SELECT id, session_id, role, content, tool_calls, tool_call_id, usage, created_at
		FROM messages WHERE session_id = $1 ORDER BY seq`, string(id))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []atom.Message
	for rows.Next() {
		var message atom.Message
		var content, calls, usage []byte
		if err := rows.Scan(&message.ID, &message.SessionID, &message.Role, &content, &calls, &message.ToolCallID, &usage, &message.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(content, &message.Content)
		_ = json.Unmarshal(calls, &message.ToolCalls)
		if len(usage) > 0 {
			_ = json.Unmarshal(usage, &message.Usage)
		}
		list = append(list, message)
	}
	return list, rows.Err()
}

type events struct{ s *Store }

func (e *events) Append(ctx context.Context, event atom.Event) error {
	_, err := e.s.pool.Exec(ctx, `
		INSERT INTO events (instance_id, session_id, name, payload, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		event.InstanceID, string(event.SessionID), string(event.Name), event.Payload, event.Time)
	return err
}

func (e *events) Since(ctx context.Context, instanceID string, seq uint64) ([]atom.Event, error) {
	rows, err := e.s.pool.Query(ctx, `
		SELECT seq, instance_id, session_id, name, payload, created_at
		FROM events WHERE seq > $1 AND ($2 = '' OR instance_id = $2) ORDER BY seq`, seq, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []atom.Event
	for rows.Next() {
		var event atom.Event
		if err := rows.Scan(&event.Seq, &event.InstanceID, &event.SessionID, &event.Name, &event.Payload, &event.Time); err != nil {
			return nil, err
		}
		list = append(list, event)
	}
	return list, rows.Err()
}

type processes struct{ s *Store }

func (p *processes) Save(ctx context.Context, record atom.ProcessRecord) error {
	spec, _ := json.Marshal(record.Spec)
	var exit any
	if record.Exit != nil {
		data, _ := json.Marshal(record.Exit)
		exit = data
	}
	_, err := p.s.pool.Exec(ctx, `
		INSERT INTO processes (id, instance_id, session_id, spec, pid, status, exit, started_at, ended_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			pid = EXCLUDED.pid,
			status = EXCLUDED.status,
			exit = EXCLUDED.exit,
			ended_at = EXCLUDED.ended_at`,
		record.ID, record.InstanceID, string(record.SessionID), spec, record.PID, record.Status, exit, record.StartedAt, record.EndedAt)
	return err
}

func (p *processes) Get(ctx context.Context, id string) (atom.ProcessRecord, error) {
	var record atom.ProcessRecord
	var spec, exit []byte
	err := p.s.pool.QueryRow(ctx, `
		SELECT id, instance_id, session_id, spec, pid, status, exit, started_at, ended_at
		FROM processes WHERE id = $1`, id).
		Scan(&record.ID, &record.InstanceID, &record.SessionID, &spec, &record.PID, &record.Status, &exit, &record.StartedAt, &record.EndedAt)
	if err != nil {
		return atom.ProcessRecord{}, err
	}
	_ = json.Unmarshal(spec, &record.Spec)
	if len(exit) > 0 {
		_ = json.Unmarshal(exit, &record.Exit)
	}
	return record, nil
}

func (p *processes) CountRunning(ctx context.Context, instanceID string) (int, error) {
	var count int
	err := p.s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM processes WHERE instance_id = $1 AND status = 'running'`, instanceID).Scan(&count)
	return count, err
}

func (p *processes) List(ctx context.Context, session atom.SessionID) ([]atom.ProcessRecord, error) {
	rows, err := p.s.pool.Query(ctx, `
		SELECT id, instance_id, session_id, spec, pid, status, exit, started_at, ended_at
		FROM processes WHERE session_id = $1 ORDER BY started_at`, string(session))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []atom.ProcessRecord
	for rows.Next() {
		var record atom.ProcessRecord
		var spec, exit []byte
		if err := rows.Scan(&record.ID, &record.InstanceID, &record.SessionID, &spec, &record.PID, &record.Status, &exit, &record.StartedAt, &record.EndedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(spec, &record.Spec)
		if len(exit) > 0 {
			_ = json.Unmarshal(exit, &record.Exit)
		}
		list = append(list, record)
	}
	return list, rows.Err()
}

type permissions struct{ s *Store }

func (p *permissions) Save(ctx context.Context, decision atom.PermissionDecision) error {
	_, err := p.s.pool.Exec(ctx, `
		INSERT INTO permissions (request_id, kind, scope, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (request_id) DO UPDATE SET kind = EXCLUDED.kind, scope = EXCLUDED.scope`,
		decision.RequestID, string(decision.Kind), string(decision.Scope), decision.CreatedAt)
	return err
}

func (p *permissions) Get(ctx context.Context, id string) (atom.PermissionDecision, error) {
	var decision atom.PermissionDecision
	err := p.s.pool.QueryRow(ctx, `
		SELECT request_id, kind, scope, created_at FROM permissions WHERE request_id = $1`, id).
		Scan(&decision.RequestID, &decision.Kind, &decision.Scope, &decision.CreatedAt)
	return decision, err
}

type usage struct{ s *Store }

func (u *usage) Save(ctx context.Context, record atom.UsageRecord) error {
	var currency string
	var value float64
	if record.Usage.Cost != nil {
		currency = record.Usage.Cost.Currency
		value = record.Usage.Cost.Value
	}
	_, err := u.s.pool.Exec(ctx, `
		INSERT INTO usage_records (instance_id, session_id, model_id, input_tokens, cache_read_tokens,
			cache_write_tokens, output_tokens, reasoning_tokens, cost_currency, cost_value, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		record.InstanceID, string(record.SessionID), record.ModelID,
		record.Usage.Input, record.Usage.CacheRead, record.Usage.CacheWrite,
		record.Usage.Output, record.Usage.Reasoning, currency, value, record.CreatedAt)
	return err
}

func (u *usage) Session(ctx context.Context, id atom.SessionID) (atom.Statistics, error) {
	return u.aggregate(ctx, `
		WITH RECURSIVE tree AS (
			SELECT id FROM sessions WHERE id = $1
			UNION ALL
			SELECT s.id FROM sessions s JOIN tree t ON s.parent_id = t.id
		)
		SELECT * FROM usage_records WHERE session_id IN (SELECT id FROM tree)`, string(id))
}

func (u *usage) Instance(ctx context.Context, id string) (atom.Statistics, error) {
	return u.aggregate(ctx, `SELECT * FROM usage_records WHERE instance_id = $1`, id)
}

func (u *usage) All(ctx context.Context) (atom.Statistics, error) {
	return u.aggregate(ctx, `SELECT * FROM usage_records`, nil)
}

func (u *usage) aggregate(ctx context.Context, query string, argument any) (atom.Statistics, error) {
	rows, err := u.s.pool.Query(ctx, `
		SELECT COUNT(*), COALESCE(SUM(input_tokens),0), COALESCE(SUM(cache_read_tokens),0),
			COALESCE(SUM(cache_write_tokens),0), COALESCE(SUM(output_tokens),0),
			COALESCE(SUM(reasoning_tokens),0)
		FROM (`+query+`) data`, argument)
	if err != nil {
		return atom.Statistics{}, err
	}
	defer rows.Close()
	var stats atom.Statistics
	if rows.Next() {
		if err := rows.Scan(&stats.Calls, &stats.Input, &stats.CacheRead, &stats.CacheWrite, &stats.Output, &stats.Reasoning); err != nil {
			return atom.Statistics{}, err
		}
	}
	if err := rows.Err(); err != nil {
		return atom.Statistics{}, err
	}
	costRows, err := u.s.pool.Query(ctx, `
		SELECT cost_currency, COALESCE(SUM(cost_value),0)
		FROM (`+query+`) data
		WHERE cost_currency <> ''
		GROUP BY cost_currency ORDER BY cost_currency`, argument)
	if err != nil {
		return atom.Statistics{}, err
	}
	defer costRows.Close()
	for costRows.Next() {
		var cost atom.Cost
		if err := costRows.Scan(&cost.Currency, &cost.Value); err != nil {
			return atom.Statistics{}, err
		}
		stats.Costs = append(stats.Costs, cost)
	}
	if err := costRows.Err(); err != nil {
		return atom.Statistics{}, err
	}
	if len(stats.Costs) == 1 {
		cost := stats.Costs[0]
		stats.Cost = &cost
	}
	return stats, nil
}

type providers struct{ s *Store }

func (p *providers) Save(ctx context.Context, spec atom.ProviderSpec) error {
	_, err := p.s.pool.Exec(ctx, `
		INSERT INTO providers (name, api_url, model_list_url, price_table_url, interval_seconds)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (name) DO UPDATE SET
			api_url = EXCLUDED.api_url,
			model_list_url = EXCLUDED.model_list_url,
			price_table_url = EXCLUDED.price_table_url,
			interval_seconds = EXCLUDED.interval_seconds`,
		spec.Name, spec.APIURL, spec.ModelListURL, spec.PriceTableURL, int64(spec.Interval/time.Second))
	return err
}

func (p *providers) Get(ctx context.Context, id string) (atom.ProviderSpec, error) {
	var spec atom.ProviderSpec
	var seconds int64
	err := p.s.pool.QueryRow(ctx, `
		SELECT name, api_url, model_list_url, price_table_url, interval_seconds
		FROM providers WHERE name = $1`, id).
		Scan(&spec.Name, &spec.APIURL, &spec.ModelListURL, &spec.PriceTableURL, &seconds)
	spec.Interval = time.Duration(seconds) * time.Second
	return spec, err
}

func (p *providers) All(ctx context.Context) ([]atom.ProviderSpec, error) {
	rows, err := p.s.pool.Query(ctx, `
		SELECT name, api_url, model_list_url, price_table_url, interval_seconds
		FROM providers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []atom.ProviderSpec
	for rows.Next() {
		var spec atom.ProviderSpec
		var seconds int64
		if err := rows.Scan(&spec.Name, &spec.APIURL, &spec.ModelListURL, &spec.PriceTableURL, &seconds); err != nil {
			return nil, err
		}
		spec.Interval = time.Duration(seconds) * time.Second
		list = append(list, spec)
	}
	return list, rows.Err()
}

func (p *providers) Delete(ctx context.Context, id string) error {
	_, err := p.s.pool.Exec(ctx, `DELETE FROM providers WHERE name = $1`, id)
	return err
}

func (p *providers) SaveModels(ctx context.Context, provider string, models []atom.ModelInfo) error {
	tx, err := p.s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM models WHERE provider = $1`, provider); err != nil {
		return err
	}
	for _, model := range models {
		input, _ := json.Marshal(model.Input)
		output, _ := json.Marshal(model.Output)
		var prices any
		if model.Prices != nil {
			data, _ := json.Marshal(model.Prices)
			prices = data
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO models (provider, id, level, input, output, tools, context_max, prices)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			provider, model.ID, model.Level, input, output, model.Tools, model.ContextMax, prices); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *providers) Models(ctx context.Context, provider string) ([]atom.ModelInfo, error) {
	rows, err := p.s.pool.Query(ctx, `
		SELECT id, level, input, output, tools, context_max, prices
		FROM models WHERE provider = $1 ORDER BY id`, provider)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []atom.ModelInfo
	for rows.Next() {
		var model atom.ModelInfo
		var input, output, prices []byte
		if err := rows.Scan(&model.ID, &model.Level, &input, &output, &model.Tools, &model.ContextMax, &prices); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(input, &model.Input)
		_ = json.Unmarshal(output, &model.Output)
		if len(prices) > 0 {
			_ = json.Unmarshal(prices, &model.Prices)
		}
		list = append(list, model)
	}
	return list, rows.Err()
}

type secrets struct{ s *Store }

func (s *secrets) SaveProviderKey(ctx context.Context, provider string, key string) error {
	_, err := s.s.pool.Exec(ctx, `
		INSERT INTO provider_keys (scope, provider, key) VALUES ('', $1, $2)
		ON CONFLICT (scope, provider) DO UPDATE SET key = EXCLUDED.key, updated_at = now()`, provider, key)
	return err
}

func (s *secrets) ProviderKey(ctx context.Context, provider string) (string, error) {
	var key string
	err := s.s.pool.QueryRow(ctx, `SELECT key FROM provider_keys WHERE scope = '' AND provider = $1`, provider).Scan(&key)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return key, err
}

func (s *secrets) SaveInstanceKey(ctx context.Context, instanceID string, provider string, key string) error {
	_, err := s.s.pool.Exec(ctx, `
		INSERT INTO provider_keys (scope, provider, key) VALUES ($1, $2, $3)
		ON CONFLICT (scope, provider) DO UPDATE SET key = EXCLUDED.key, updated_at = now()`, instanceID, provider, key)
	return err
}

func (s *secrets) InstanceKey(ctx context.Context, instanceID string, provider string) (string, error) {
	var key string
	err := s.s.pool.QueryRow(ctx, `SELECT key FROM provider_keys WHERE scope = $1 AND provider = $2`, instanceID, provider).Scan(&key)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return key, err
}

func (s *secrets) ResolveKey(ctx context.Context, instanceID string, provider string) (string, error) {
	if instanceID != "" {
		value, err := s.InstanceKey(ctx, instanceID, provider)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
	}
	return s.ProviderKey(ctx, provider)
}

func (s *secrets) DeleteProviderKey(ctx context.Context, provider string) error {
	_, err := s.s.pool.Exec(ctx, `DELETE FROM provider_keys WHERE scope = '' AND provider = $1`, provider)
	return err
}

func (s *secrets) DeleteInstanceKey(ctx context.Context, instanceID string, provider string) error {
	_, err := s.s.pool.Exec(ctx, `DELETE FROM provider_keys WHERE scope = $1 AND provider = $2`, instanceID, provider)
	return err
}

type settings struct{ s *Store }

func (s *settings) Save(ctx context.Context, scope string, key string, value string) error {
	_, err := s.s.pool.Exec(ctx, `
		INSERT INTO settings (scope, key, value) VALUES ($1, $2, $3)
		ON CONFLICT (scope, key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, scope, key, value)
	return err
}

func (s *settings) Get(ctx context.Context, scope string, key string) (string, error) {
	var value string
	err := s.s.pool.QueryRow(ctx, `SELECT value FROM settings WHERE scope = $1 AND key = $2`, scope, key).Scan(&value)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *settings) All(ctx context.Context, scope string) (map[string]string, error) {
	rows, err := s.s.pool.Query(ctx, `SELECT key, value FROM settings WHERE scope = $1 ORDER BY key`, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, rows.Err()
}

func (s *settings) Delete(ctx context.Context, scope string, key string) error {
	_, err := s.s.pool.Exec(ctx, `DELETE FROM settings WHERE scope = $1 AND key = $2`, scope, key)
	return err
}

func (s *settings) Resolve(ctx context.Context, instanceID string, key string) (string, error) {
	if instanceID != "" {
		value, err := s.Get(ctx, instanceID, key)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
	}
	return s.Get(ctx, "", key)
}
