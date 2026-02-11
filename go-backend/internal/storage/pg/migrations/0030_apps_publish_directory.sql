-- +goose Up
ALTER TABLE apps ADD COLUMN publish_directory TEXT;

-- +goose Down
ALTER TABLE apps DROP COLUMN IF EXISTS publish_directory;
