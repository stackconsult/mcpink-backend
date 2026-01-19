-- +goose Up

-- Add Fly.io credentials to users table
ALTER TABLE users ADD COLUMN flyio_token TEXT;
ALTER TABLE users ADD COLUMN flyio_org TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS flyio_token;
ALTER TABLE users DROP COLUMN IF EXISTS flyio_org;
