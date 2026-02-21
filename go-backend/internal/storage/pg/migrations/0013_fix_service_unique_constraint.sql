-- +goose Up
ALTER TABLE services DROP CONSTRAINT services_project_id_name_key;
CREATE UNIQUE INDEX services_project_id_name_key ON services(project_id, name) WHERE is_deleted = false;

-- +goose Down
DROP INDEX services_project_id_name_key;
ALTER TABLE services ADD CONSTRAINT services_project_id_name_key UNIQUE (project_id, name);
