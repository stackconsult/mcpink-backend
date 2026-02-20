-- +goose Up
ALTER TABLE deployments DROP CONSTRAINT valid_status;
ALTER TABLE deployments ADD CONSTRAINT valid_status CHECK (
    status IN ('queued','building','deploying','active','failed','cancelled','superseded','crashed','completed','removed')
);

-- +goose Down
-- Cannot safely shrink if rows exist with new statuses
