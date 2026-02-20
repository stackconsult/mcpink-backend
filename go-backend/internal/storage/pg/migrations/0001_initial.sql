-- +goose Up

-- Users
CREATE TABLE users (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    github_id BIGINT UNIQUE,
    email TEXT UNIQUE,
    firebase_uid TEXT UNIQUE,
    github_username TEXT,
    gitea_username TEXT UNIQUE,
    avatar_url TEXT,
    display_name TEXT,
    github_scopes TEXT[] DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- API Keys
CREATE TABLE api_keys (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    key_prefix TEXT NOT NULL UNIQUE,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Projects
CREATE TABLE projects (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    ref TEXT NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, ref)
);

-- GitHub Credentials
CREATE TABLE github_creds (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    github_id BIGINT,
    github_oauth_token TEXT,
    github_oauth_scopes TEXT[],
    github_oauth_updated_at TIMESTAMPTZ,
    github_app_installation_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Internal Repos (Gitea)
CREATE TABLE internal_repos (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    clone_url TEXT NOT NULL,
    provider TEXT NOT NULL DEFAULT 'gitea',
    repo_id TEXT,
    full_name TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Resources (databases, etc.)
CREATE TABLE resources (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'sqlite',
    provider TEXT NOT NULL DEFAULT 'turso',
    region TEXT NOT NULL DEFAULT 'eu-central',
    external_id TEXT,
    connection_url TEXT,
    auth_token TEXT,
    credentials JSONB,
    metadata JSONB,
    status TEXT NOT NULL DEFAULT 'provisioning',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Services (long-lived entity — NO deployment state here)
CREATE TABLE services (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    -- Git source
    repo TEXT NOT NULL,
    branch TEXT NOT NULL,
    git_provider TEXT NOT NULL DEFAULT 'github',

    -- Service config (mutable — next deploy snapshots these)
    name TEXT,
    port TEXT NOT NULL DEFAULT '3000',
    build_pack TEXT NOT NULL DEFAULT 'railpack',
    env_vars JSONB DEFAULT '[]'::JSONB,
    build_config JSONB,
    memory TEXT NOT NULL DEFAULT '256Mi',
    vcpus TEXT NOT NULL DEFAULT '0.5',
    publish_directory TEXT,

    -- Stable identifiers
    fqdn TEXT,
    custom_domain TEXT,
    server_uuid TEXT NOT NULL DEFAULT 'k8s',

    -- Points to the currently-live deployment
    current_deployment_id TEXT,

    -- Lifecycle
    is_deleted BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(project_id, name)
);

CREATE INDEX idx_services_user_id ON services(user_id);
CREATE INDEX idx_services_project_id ON services(project_id);

-- Deployments (immutable event per deploy attempt)
CREATE TABLE deployments (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,

    -- Temporal correlation
    workflow_id TEXT NOT NULL UNIQUE,
    workflow_run_id TEXT,

    -- Config snapshot (immutable after creation)
    commit_hash TEXT,
    image_ref TEXT,
    build_pack TEXT NOT NULL,
    build_config JSONB NOT NULL DEFAULT '{}'::JSONB,
    env_vars_snapshot JSONB NOT NULL DEFAULT '[]'::JSONB,
    memory TEXT NOT NULL,
    vcpus TEXT NOT NULL,
    port TEXT NOT NULL,

    -- Status
    status TEXT NOT NULL DEFAULT 'queued',
    error_message TEXT,
    build_progress JSONB,

    -- Trigger metadata
    trigger TEXT NOT NULL DEFAULT 'manual',
    trigger_ref TEXT,

    -- Timestamps
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT valid_status CHECK (
        status IN ('queued','building','deploying','active','failed','cancelled','superseded')
    ),
    CONSTRAINT terminal_has_finished_at CHECK (
        (status NOT IN ('failed','cancelled','superseded','active')) OR finished_at IS NOT NULL
    ),
    CONSTRAINT nonterminal_no_finished_at CHECK (
        (status NOT IN ('queued','building','deploying')) OR finished_at IS NULL
    )
);

CREATE INDEX idx_deployments_service_id ON deployments(service_id);
CREATE INDEX idx_deployments_service_created ON deployments(service_id, created_at DESC);
CREATE INDEX idx_deployments_workflow_id ON deployments(workflow_id);
CREATE UNIQUE INDEX idx_deployments_one_active ON deployments(service_id) WHERE status = 'active';
CREATE UNIQUE INDEX idx_deployments_one_inflight ON deployments(service_id) WHERE status IN ('queued','building','deploying');

-- FK from services to deployments
ALTER TABLE services ADD CONSTRAINT fk_services_current_deployment
    FOREIGN KEY (current_deployment_id) REFERENCES deployments(id);

-- DNS Records
CREATE TABLE dns_records (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    cloudflare_record_id TEXT,
    subdomain TEXT NOT NULL,
    full_domain TEXT NOT NULL,
    target_ip TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'A',
    name TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    proxied BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_dns_records_service_id ON dns_records(service_id);

-- Custom Domains
CREATE TABLE custom_domains (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    domain TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending_dns',
    expected_record_target TEXT NOT NULL,
    expires_at TIMESTAMPTZ DEFAULT (NOW() + INTERVAL '7 days'),
    verified_at TIMESTAMPTZ,
    last_checked_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(service_id),
    UNIQUE(domain)
);

-- +goose Down
DROP TABLE IF EXISTS custom_domains;
DROP TABLE IF EXISTS dns_records;
ALTER TABLE services DROP CONSTRAINT IF EXISTS fk_services_current_deployment;
DROP TABLE IF EXISTS deployments;
DROP TABLE IF EXISTS services;
DROP TABLE IF EXISTS resources;
DROP TABLE IF EXISTS internal_repos;
DROP TABLE IF EXISTS github_creds;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS users;
