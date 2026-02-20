-- name: CreateHostedZone :one
INSERT INTO hosted_zones (id, user_id, zone, verification_token)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetHostedZoneByID :one
SELECT * FROM hosted_zones WHERE id = $1;

-- name: GetHostedZoneByZone :one
SELECT * FROM hosted_zones WHERE lower(zone) = lower($1);

-- name: ListHostedZonesByUserID :many
SELECT * FROM hosted_zones
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: DeleteHostedZone :exec
DELETE FROM hosted_zones WHERE id = $1;

-- name: UpdateHostedZoneTXTVerified :one
UPDATE hosted_zones
SET status = 'pending_delegation',
    verified_at = NOW(),
    last_error = NULL,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateHostedZoneProvisioning :one
UPDATE hosted_zones
SET status = 'provisioning',
    delegated_at = NOW(),
    last_error = NULL,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateHostedZoneActivated :one
UPDATE hosted_zones
SET status = 'active',
    wildcard_cert_secret = $2,
    cert_issued_at = NOW(),
    last_error = NULL,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateHostedZoneStatus :one
UPDATE hosted_zones
SET status = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateHostedZoneError :exec
UPDATE hosted_zones
SET last_error = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: FindOverlappingHostedZone :one
SELECT * FROM hosted_zones
WHERE status != 'failed'
  AND (lower(zone) = lower($1)
       OR lower($1) LIKE '%.' || lower(zone)
       OR lower(zone) LIKE '%.' || lower($1))
LIMIT 1;

-- name: FindMatchingHostedZoneForDomain :one
SELECT * FROM hosted_zones
WHERE user_id = $1
  AND status = 'active'
  AND lower($2) LIKE '%.' || lower(zone)
ORDER BY length(zone) DESC
LIMIT 1;
