-- +goose Up
-- Soft-delete duplicate active apps (keep the newest per name+project)
UPDATE apps SET is_deleted = true
WHERE id IN (
    SELECT id FROM (
        SELECT id, ROW_NUMBER() OVER (PARTITION BY name, project_id ORDER BY created_at DESC) AS rn
        FROM apps WHERE is_deleted = false AND name IS NOT NULL
    ) sub WHERE rn > 1
);
CREATE UNIQUE INDEX idx_apps_name_project_active ON apps(name, project_id) WHERE is_deleted = false;

-- +goose Down
DROP INDEX IF EXISTS idx_apps_name_project_active;
