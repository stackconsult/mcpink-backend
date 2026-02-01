-- +goose Up
ALTER TABLE users ADD COLUMN coolify_github_app_uuid TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN coolify_github_app_uuid;
