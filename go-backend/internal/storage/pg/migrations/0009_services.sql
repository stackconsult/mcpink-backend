-- +goose Up
CREATE TABLE services (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    coolify_app_uuid TEXT,

    -- Two separate status fields
    build_status TEXT NOT NULL DEFAULT 'queued',
    runtime_status TEXT,
    error_message TEXT,

    repo TEXT NOT NULL,
    branch TEXT NOT NULL,
    server_uuid TEXT NOT NULL,
    name TEXT,
    build_pack TEXT NOT NULL DEFAULT 'nixpacks',
    port TEXT NOT NULL DEFAULT '3000',
    env_vars JSONB DEFAULT '[]'::JSONB,
    fqdn TEXT,
    workflow_id TEXT NOT NULL UNIQUE,
    workflow_run_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_services_user_id ON services(user_id);
CREATE INDEX idx_services_build_status ON services(build_status);
CREATE INDEX idx_services_workflow_id ON services(workflow_id);

-- +goose Down
DROP TABLE IF EXISTS services;
