-- +goose Up
ALTER TABLE apps DROP CONSTRAINT apps_workflow_id_key;

-- +goose Down
ALTER TABLE apps ADD CONSTRAINT apps_workflow_id_key UNIQUE (workflow_id);
