-- +goose Up
ALTER TABLE dns_records RENAME COLUMN app_id TO service_id;
ALTER INDEX idx_dns_records_app_id RENAME TO idx_dns_records_service_id;

-- +goose Down
ALTER TABLE dns_records RENAME COLUMN service_id TO app_id;
ALTER INDEX idx_dns_records_service_id RENAME TO idx_dns_records_app_id;
