-- +goose Up
ALTER TABLE github_creds DROP COLUMN IF EXISTS github_oauth_refresh_token;

-- +goose Down
ALTER TABLE github_creds ADD COLUMN github_oauth_refresh_token TEXT;
