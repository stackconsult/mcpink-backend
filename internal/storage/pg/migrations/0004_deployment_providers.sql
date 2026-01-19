-- +goose Up
CREATE TABLE deployment_providers (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_type TEXT NOT NULL,
    encrypted_token TEXT NOT NULL,
    organization TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, provider_type)
);

CREATE INDEX idx_deployment_providers_user_id ON deployment_providers(user_id);

-- Migrate existing Fly.io credentials from users table
INSERT INTO deployment_providers (user_id, provider_type, encrypted_token, organization, metadata)
SELECT id, 'flyio', flyio_token, flyio_org, '{}'::jsonb
FROM users
WHERE flyio_token IS NOT NULL AND flyio_token != '';

-- +goose Down
DROP TABLE IF EXISTS deployment_providers;
