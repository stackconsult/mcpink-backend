-- +goose Up
ALTER TABLE apps ADD COLUMN is_deleted BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE apps DROP COLUMN is_deleted;
