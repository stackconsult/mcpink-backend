-- +goose Up
ALTER TABLE apps ADD COLUMN commit_hash TEXT;

-- +goose Down
ALTER TABLE apps DROP COLUMN commit_hash;
