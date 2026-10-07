package contextplugin

const databaseSchema = `
CREATE EXTENSION IF NOT EXISTS vector;
CREATE SCHEMA IF NOT EXISTS context_plugin;
CREATE SEQUENCE IF NOT EXISTS context_plugin.source_ids;
CREATE TABLE IF NOT EXISTS context_plugin.documents (
    workspace_id text NOT NULL,
    kind text NOT NULL,
    key text NOT NULL,
    data jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id,kind,key)
);
CREATE INDEX IF NOT EXISTS context_jobs_status ON context_plugin.documents ((data->>'status'),updated_at) WHERE kind='job';
CREATE TABLE IF NOT EXISTS context_plugin.vectors (
    workspace_id text NOT NULL,
    kind text NOT NULL,
    key text NOT NULL,
    version int NOT NULL,
    chunk int NOT NULL,
    model text NOT NULL,
    categories text[] NOT NULL DEFAULT '{}',
    content text NOT NULL,
    embedding vector NOT NULL,
    deleted boolean NOT NULL DEFAULT false,
    historical boolean NOT NULL DEFAULT false,
    search_text tsvector GENERATED ALWAYS AS (to_tsvector('simple',content)) STORED,
    PRIMARY KEY(workspace_id,kind,key,version,chunk)
);
CREATE INDEX IF NOT EXISTS context_vectors_scope ON context_plugin.vectors (workspace_id,kind,deleted,historical,model);
CREATE INDEX IF NOT EXISTS context_vectors_text ON context_plugin.vectors USING gin(search_text);
CREATE INDEX IF NOT EXISTS context_vectors_categories ON context_plugin.vectors USING gin(categories);
CREATE TABLE IF NOT EXISTS context_plugin.metrics (
    workspace_id text NOT NULL,
    agent text NOT NULL,
    name text NOT NULL,
    value bigint NOT NULL DEFAULT 0,
    duration_ms bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(workspace_id,agent,name)
);
`
