-- +goose Up
-- +goose StatementBegin
DO $$ BEGIN
  -- Table rename (idempotent: skip if already renamed by a partial prior run)
  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'apps' AND relkind = 'r') THEN
    ALTER TABLE apps RENAME TO services;
  END IF;

  -- Index renames (skip each if already renamed)
  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_apps_user_id') THEN
    ALTER INDEX idx_apps_user_id RENAME TO idx_services_user_id;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_apps_build_status') THEN
    ALTER INDEX idx_apps_build_status RENAME TO idx_services_build_status;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_apps_workflow_id') THEN
    ALTER INDEX idx_apps_workflow_id RENAME TO idx_services_workflow_id;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_apps_project_id') THEN
    ALTER INDEX idx_apps_project_id RENAME TO idx_services_project_id;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_apps_name_project_active') THEN
    ALTER INDEX idx_apps_name_project_active RENAME TO idx_services_name_project_active;
  END IF;

  -- Constraint rename (skip if already renamed)
  IF EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'apps_user_id_fkey'
      AND conrelid = 'services'::regclass
  ) THEN
    ALTER TABLE services RENAME CONSTRAINT apps_user_id_fkey TO services_user_id_fkey;
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'services' AND relkind = 'r') THEN
    ALTER TABLE services RENAME TO apps;
  END IF;

  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_services_user_id') THEN
    ALTER INDEX idx_services_user_id RENAME TO idx_apps_user_id;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_services_build_status') THEN
    ALTER INDEX idx_services_build_status RENAME TO idx_apps_build_status;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_services_workflow_id') THEN
    ALTER INDEX idx_services_workflow_id RENAME TO idx_apps_workflow_id;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_services_project_id') THEN
    ALTER INDEX idx_services_project_id RENAME TO idx_apps_project_id;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'idx_services_name_project_active') THEN
    ALTER INDEX idx_services_name_project_active RENAME TO idx_apps_name_project_active;
  END IF;

  IF EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'services_user_id_fkey'
      AND conrelid = 'apps'::regclass
  ) THEN
    ALTER TABLE apps RENAME CONSTRAINT services_user_id_fkey TO apps_user_id_fkey;
  END IF;
END $$;
-- +goose StatementEnd
