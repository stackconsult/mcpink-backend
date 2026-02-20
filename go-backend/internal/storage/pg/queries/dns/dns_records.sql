-- name: CreateDnsRecord :one
INSERT INTO dns_records (id, zone_id, name, rrtype, content, ttl, managed, service_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: DeleteDnsRecord :exec
DELETE FROM dns_records WHERE id = $1;

-- name: GetDnsRecordByID :one
SELECT * FROM dns_records WHERE id = $1;

-- name: ListDnsRecordsByZoneID :many
SELECT * FROM dns_records
WHERE zone_id = $1
ORDER BY name, rrtype;

-- name: ListDnsRecordsByZoneAndName :many
SELECT * FROM dns_records
WHERE zone_id = $1 AND lower(name) = lower($2);

-- name: ListDnsRecordsByServiceID :many
SELECT * FROM dns_records
WHERE service_id = $1
ORDER BY created_at DESC;

-- name: ListCustomDomainsByServiceIDs :many
SELECT dr.service_id, dr.name, hz.zone, hz.status
FROM dns_records dr
JOIN hosted_zones hz ON dr.zone_id = hz.id
WHERE dr.service_id = ANY($1::text[])
  AND dr.service_id IS NOT NULL;
