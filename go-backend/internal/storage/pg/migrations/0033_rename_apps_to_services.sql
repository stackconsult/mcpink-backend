-- +goose Up
ALTER TABLE apps RENAME TO services;
ALTER INDEX idx_apps_user_id RENAME TO idx_services_user_id;
ALTER INDEX idx_apps_build_status RENAME TO idx_services_build_status;
ALTER INDEX idx_apps_workflow_id RENAME TO idx_services_workflow_id;
ALTER INDEX idx_apps_project_id RENAME TO idx_services_project_id;
ALTER INDEX idx_apps_name_project_active RENAME TO idx_services_name_project_active;
ALTER TABLE services RENAME CONSTRAINT apps_user_id_fkey TO services_user_id_fkey;

-- +goose Down
ALTER TABLE services RENAME TO apps;
ALTER INDEX idx_services_user_id RENAME TO idx_apps_user_id;
ALTER INDEX idx_services_build_status RENAME TO idx_apps_build_status;
ALTER INDEX idx_services_workflow_id RENAME TO idx_apps_workflow_id;
ALTER INDEX idx_services_project_id RENAME TO idx_apps_project_id;
ALTER INDEX idx_services_name_project_active RENAME TO idx_apps_name_project_active;
ALTER TABLE apps RENAME CONSTRAINT services_user_id_fkey TO apps_user_id_fkey;
