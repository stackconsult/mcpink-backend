-- +goose Up

-- Rename delegated_zones → hosted_zones
ALTER TABLE delegated_zones RENAME TO hosted_zones;
ALTER INDEX idx_delegated_zones_user_id RENAME TO idx_hosted_zones_user_id;
ALTER TABLE hosted_zones RENAME CONSTRAINT valid_zone_status TO valid_hz_status;

-- Create dns_records table (replaces zone_records)
CREATE TABLE dns_records (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    zone_id TEXT NOT NULL REFERENCES hosted_zones(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    rrtype TEXT NOT NULL DEFAULT 'A',
    content TEXT NOT NULL,
    ttl INT NOT NULL DEFAULT 300,
    managed BOOLEAN NOT NULL DEFAULT false,
    service_id TEXT REFERENCES services(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(zone_id, name, rrtype, content)
);
CREATE INDEX idx_dns_records_zone_id ON dns_records(zone_id);
CREATE INDEX idx_dns_records_service_id ON dns_records(service_id);

-- Migrate existing zone_records → dns_records (A records, managed, with ingress_ip content)
INSERT INTO dns_records (id, zone_id, name, rrtype, content, ttl, managed, service_id, created_at)
SELECT
    zr.id,
    zr.zone_id,
    zr.name,
    'A',
    c.ingress_ip,
    300,
    true,
    zr.service_id,
    zr.created_at
FROM zone_records zr
JOIN hosted_zones hz ON zr.zone_id = hz.id
JOIN services s ON zr.service_id = s.id
JOIN clusters c ON c.region = s.region;

-- Drop old zone_records
DROP TABLE zone_records;

-- +goose Down
-- Recreate zone_records from dns_records
CREATE TABLE zone_records (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    zone_id TEXT NOT NULL REFERENCES hosted_zones(id) ON DELETE CASCADE,
    service_id TEXT NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(zone_id, name)
);
CREATE INDEX idx_zone_records_service_id ON zone_records(service_id);

INSERT INTO zone_records (id, zone_id, service_id, name, created_at)
SELECT id, zone_id, service_id, name, created_at
FROM dns_records
WHERE managed = true AND service_id IS NOT NULL;

DROP TABLE dns_records;

ALTER TABLE hosted_zones RENAME TO delegated_zones;
ALTER INDEX idx_hosted_zones_user_id RENAME TO idx_delegated_zones_user_id;
ALTER TABLE delegated_zones RENAME CONSTRAINT valid_hz_status TO valid_zone_status;
