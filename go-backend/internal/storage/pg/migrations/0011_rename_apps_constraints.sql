-- +goose Up
ALTER TABLE apps RENAME CONSTRAINT services_user_id_fkey TO apps_user_id_fkey;
ALTER TABLE apps RENAME CONSTRAINT services_workflow_id_key TO apps_workflow_id_key;

-- +goose Down
ALTER TABLE apps RENAME CONSTRAINT apps_user_id_fkey TO services_user_id_fkey;
ALTER TABLE apps RENAME CONSTRAINT apps_workflow_id_key TO services_workflow_id_key;
