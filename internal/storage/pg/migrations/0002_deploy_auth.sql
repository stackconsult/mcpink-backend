-- +goose Up

-- Update users table for GitHub OAuth
ALTER TABLE users DROP COLUMN IF EXISTS email;
ALTER TABLE users DROP COLUMN IF EXISTS name;

ALTER TABLE users ADD COLUMN github_id BIGINT UNIQUE NOT NULL;
ALTER TABLE users ADD COLUMN github_username TEXT NOT NULL;
ALTER TABLE users ADD COLUMN github_token TEXT NOT NULL;
ALTER TABLE users ADD COLUMN avatar_url TEXT;

CREATE INDEX idx_users_github_id ON users(github_id);

-- API keys table
CREATE TABLE api_keys (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX idx_api_keys_key_prefix ON api_keys(key_prefix);

-- +goose Down
DROP TABLE IF EXISTS api_keys;
ALTER TABLE users DROP COLUMN IF EXISTS github_id;
ALTER TABLE users DROP COLUMN IF EXISTS github_username;
ALTER TABLE users DROP COLUMN IF EXISTS github_token;
ALTER TABLE users DROP COLUMN IF EXISTS avatar_url;
ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN name TEXT NOT NULL DEFAULT '';
