-- +goose Up

ALTER TABLE internal_repos ADD COLUMN project_id TEXT REFERENCES projects(id) ON DELETE CASCADE;

-- Backfill: assign existing repos to the user's default project
UPDATE internal_repos r SET project_id = (
    SELECT p.id FROM projects p WHERE p.user_id = r.user_id AND p.is_default = true LIMIT 1
);

-- For repos whose user has no default project, create one
INSERT INTO projects (id, user_id, name, ref, is_default)
SELECT gen_random_uuid()::TEXT, r.user_id, 'Default', 'default', true
FROM internal_repos r
WHERE r.project_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM projects p WHERE p.user_id = r.user_id AND p.is_default = true)
GROUP BY r.user_id;

-- Retry backfill after creating default projects
UPDATE internal_repos r SET project_id = (
    SELECT p.id FROM projects p WHERE p.user_id = r.user_id AND p.is_default = true LIMIT 1
)
WHERE r.project_id IS NULL;

ALTER TABLE internal_repos ALTER COLUMN project_id SET NOT NULL;

CREATE UNIQUE INDEX idx_internal_repos_project_name ON internal_repos(project_id, name);

-- +goose Down

DROP INDEX IF EXISTS idx_internal_repos_project_name;
ALTER TABLE internal_repos DROP COLUMN IF EXISTS project_id;
