-- +goose Up

-- Create projects table
CREATE TABLE projects (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, name)
);

CREATE INDEX idx_projects_user_id ON projects(user_id);

-- Delete all existing apps for clean slate
DELETE FROM apps;

-- Add project_id to apps
ALTER TABLE apps ADD COLUMN project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE;
CREATE INDEX idx_apps_project_id ON apps(project_id);

-- +goose Down
ALTER TABLE apps DROP COLUMN project_id;
DROP TABLE IF EXISTS projects;
