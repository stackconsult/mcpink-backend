-- +goose Up
ALTER TABLE services RENAME TO apps;
ALTER INDEX idx_services_user_id RENAME TO idx_apps_user_id;
ALTER INDEX idx_services_build_status RENAME TO idx_apps_build_status;
ALTER INDEX idx_services_workflow_id RENAME TO idx_apps_workflow_id;

-- +goose Down
ALTER TABLE apps RENAME TO services;
ALTER INDEX idx_apps_user_id RENAME TO idx_services_user_id;
ALTER INDEX idx_apps_build_status RENAME TO idx_services_build_status;
ALTER INDEX idx_apps_workflow_id RENAME TO idx_services_workflow_id;
