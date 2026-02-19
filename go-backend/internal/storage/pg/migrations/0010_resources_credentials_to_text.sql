-- +goose Up
ALTER TABLE resources ALTER COLUMN credentials TYPE TEXT USING credentials::TEXT;

-- +goose Down
ALTER TABLE resources ALTER COLUMN credentials TYPE JSONB USING credentials::JSONB;
