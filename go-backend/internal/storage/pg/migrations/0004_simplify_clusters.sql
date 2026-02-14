-- +goose Up

-- Drop redundant region column (id already serves as the region identifier)
ALTER TABLE clusters DROP COLUMN region;

-- Rename cluster_id → region in services to match user-facing terminology
ALTER TABLE services DROP CONSTRAINT services_cluster_id_fkey;
DROP INDEX idx_services_cluster_id;
ALTER TABLE services RENAME COLUMN cluster_id TO region;

-- Rename id → region in clusters
ALTER TABLE clusters RENAME COLUMN id TO region;

-- Re-add FK and index with new names
ALTER TABLE services ADD CONSTRAINT services_region_fkey
    FOREIGN KEY (region) REFERENCES clusters(region);
CREATE INDEX idx_services_region ON services(region);

-- +goose Down
DROP INDEX IF EXISTS idx_services_region;
ALTER TABLE services DROP CONSTRAINT IF EXISTS services_region_fkey;

ALTER TABLE clusters RENAME COLUMN region TO id;
ALTER TABLE services RENAME COLUMN region TO cluster_id;

ALTER TABLE clusters ADD COLUMN region TEXT NOT NULL DEFAULT '';
UPDATE clusters SET region = 'eu-central' WHERE id = 'eu-central-1';

ALTER TABLE services ADD CONSTRAINT services_cluster_id_fkey
    FOREIGN KEY (cluster_id) REFERENCES clusters(id);
CREATE INDEX idx_services_cluster_id ON services(cluster_id);
