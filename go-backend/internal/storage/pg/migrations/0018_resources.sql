-- +goose Up
CREATE TABLE resources (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,              -- 'sqlite', 'postgres', 'mongo', 'llm'
    provider TEXT NOT NULL,          -- 'turso', 'neon', 'atlas', 'openai'
    region TEXT NOT NULL,            -- 'eu-west', etc.
    external_id TEXT,                -- Provider's ID (Turso's DbId)
    credentials TEXT NOT NULL,       -- Encrypted JSON: {url, auth_token}
    metadata JSONB NOT NULL DEFAULT '{}',  -- Non-sensitive: {size, hostname, group}
    status TEXT NOT NULL DEFAULT 'provisioning',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, name)
);

CREATE INDEX idx_resources_user_id ON resources(user_id);
CREATE INDEX idx_resources_project_id ON resources(project_id);
CREATE INDEX idx_resources_type ON resources(type);
CREATE INDEX idx_resources_status ON resources(status);

-- +goose Down
DROP TABLE IF EXISTS resources;
