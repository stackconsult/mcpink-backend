-- +goose Up
CREATE TABLE clusters (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    region        TEXT NOT NULL,
    task_queue    TEXT NOT NULL UNIQUE,
    apps_domain   TEXT NOT NULL,
    cname_target  TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO clusters (id, name, region, task_queue, apps_domain, cname_target)
VALUES ('eu-central-1', 'Europe Central 1', 'eu-central', 'deployer-eu-central-1', 'ml.ink', 'cname.ml.ink');

ALTER TABLE services ADD COLUMN cluster_id TEXT NOT NULL
    REFERENCES clusters(id) DEFAULT 'eu-central-1';
CREATE INDEX idx_services_cluster_id ON services(cluster_id);

-- +goose Down
DROP INDEX IF EXISTS idx_services_cluster_id;
ALTER TABLE services DROP COLUMN IF EXISTS cluster_id;
DROP TABLE IF EXISTS clusters;
