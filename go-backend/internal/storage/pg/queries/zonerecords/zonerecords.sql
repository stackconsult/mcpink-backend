-- name: Create :one
INSERT INTO zone_records (zone_id, service_id, name)
VALUES ($1, $2, $3) RETURNING *;

-- name: Delete :exec
DELETE FROM zone_records WHERE id = $1;

-- name: GetByZoneAndName :one
SELECT * FROM zone_records
WHERE zone_id = $1 AND lower(name) = lower($2);

-- name: ListByZoneID :many
SELECT * FROM zone_records
WHERE zone_id = $1
ORDER BY created_at DESC;

-- name: ListByServiceID :many
SELECT * FROM zone_records
WHERE service_id = $1
ORDER BY created_at DESC;

-- name: DeleteByServiceID :exec
DELETE FROM zone_records WHERE service_id = $1;

-- name: ListByServiceIDs :many
SELECT * FROM zone_records
WHERE service_id = ANY($1::text[])
ORDER BY created_at DESC;
