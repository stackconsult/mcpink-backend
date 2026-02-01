-- +goose Up

-- Create github_creds table
CREATE TABLE github_creds (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    github_id BIGINT NOT NULL,
    github_oauth_token TEXT NOT NULL,
    github_oauth_scopes TEXT[] NOT NULL DEFAULT '{}',
    github_oauth_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    github_app_installation_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_github_creds_user_id ON github_creds(user_id);
CREATE INDEX idx_github_creds_github_id ON github_creds(github_id);
CREATE INDEX idx_github_creds_github_app_installation_id ON github_creds(github_app_installation_id);

-- Migrate existing data from users table
INSERT INTO github_creds (user_id, github_id, github_oauth_token, github_oauth_scopes, github_app_installation_id)
SELECT id, github_id, github_token, github_scopes, github_app_installation_id
FROM users
WHERE github_token IS NOT NULL AND github_token != '';

-- Drop moved columns from users table
ALTER TABLE users DROP COLUMN IF EXISTS github_token;
ALTER TABLE users DROP COLUMN IF EXISTS github_scopes;
ALTER TABLE users DROP COLUMN IF EXISTS github_app_installation_id;

-- +goose Down

-- Add columns back to users
ALTER TABLE users ADD COLUMN github_token TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN github_scopes TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE users ADD COLUMN github_app_installation_id BIGINT;

-- Migrate data back
UPDATE users u
SET github_token = gc.github_oauth_token,
    github_scopes = gc.github_oauth_scopes,
    github_app_installation_id = gc.github_app_installation_id
FROM github_creds gc
WHERE u.id = gc.user_id;

-- Remove default from github_token
ALTER TABLE users ALTER COLUMN github_token DROP DEFAULT;

-- Drop the github_creds table
DROP TABLE IF EXISTS github_creds;
