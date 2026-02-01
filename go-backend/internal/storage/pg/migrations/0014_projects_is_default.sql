-- +goose Up
ALTER TABLE projects ADD COLUMN is_default BOOLEAN NOT NULL DEFAULT FALSE;

-- Set existing 'default' named projects as default
UPDATE projects SET is_default = TRUE WHERE name = 'default';

-- Create unique partial index: only one default per user
CREATE UNIQUE INDEX idx_projects_user_default ON projects(user_id) WHERE is_default = TRUE;

-- +goose Down
DROP INDEX IF EXISTS idx_projects_user_default;
ALTER TABLE projects DROP COLUMN is_default;
