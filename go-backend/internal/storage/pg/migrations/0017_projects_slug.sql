-- +goose Up
ALTER TABLE projects ADD COLUMN ref TEXT;

-- Generate refs for existing projects
UPDATE projects SET ref = LOWER(REGEXP_REPLACE(REGEXP_REPLACE(name, '[^a-zA-Z0-9\s-]', '', 'g'), '\s+', '-', 'g'));

-- Make ref NOT NULL after populating
ALTER TABLE projects ALTER COLUMN ref SET NOT NULL;

-- Drop old unique constraint, add new one
ALTER TABLE projects DROP CONSTRAINT projects_user_id_name_key;
CREATE UNIQUE INDEX idx_projects_user_ref ON projects(user_id, ref);

-- +goose Down
DROP INDEX IF EXISTS idx_projects_user_ref;
ALTER TABLE projects ADD CONSTRAINT projects_user_id_name_key UNIQUE (user_id, name);
ALTER TABLE projects DROP COLUMN ref;
