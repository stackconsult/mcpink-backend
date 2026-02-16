-- name: Create :one
INSERT INTO delegated_zones (user_id, zone, verification_token)
VALUES ($1, $2, $3) RETURNING *;

-- name: GetByID :one
SELECT * FROM delegated_zones WHERE id = $1;

-- name: GetByZone :one
SELECT * FROM delegated_zones WHERE lower(zone) = lower($1);

-- name: ListByUserID :many
SELECT * FROM delegated_zones
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: Delete :exec
DELETE FROM delegated_zones WHERE id = $1;

-- name: UpdateTXTVerified :one
UPDATE delegated_zones
SET status = 'pending_delegation',
    verified_at = NOW(),
    last_error = NULL,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateProvisioning :one
UPDATE delegated_zones
SET status = 'provisioning',
    delegated_at = NOW(),
    last_error = NULL,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateActivated :one
UPDATE delegated_zones
SET status = 'active',
    wildcard_cert_secret = $2,
    cert_issued_at = NOW(),
    last_error = NULL,
    updated_at = NOW()
WHERE id = $1
RETURNING *;


-- name: UpdateStatus :one
UPDATE delegated_zones
SET status = $2,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateError :exec
UPDATE delegated_zones
SET last_error = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: FindOverlappingZone :one
SELECT * FROM delegated_zones
WHERE status != 'failed'
  AND (lower(zone) = lower($1)
       OR lower($1) LIKE '%.' || lower(zone)
       OR lower(zone) LIKE '%.' || lower($1))
LIMIT 1;

-- name: FindMatchingZoneForDomain :one
SELECT * FROM delegated_zones
WHERE user_id = $1
  AND status = 'active'
  AND lower($2) LIKE '%.' || lower(zone)
ORDER BY length(zone) DESC
LIMIT 1;

-- name: GetByIDs :many
SELECT * FROM delegated_zones WHERE id = ANY($1::text[]);

-- name: ExpireStale :exec
UPDATE delegated_zones
SET status = 'failed',
    last_error = 'expired',
    updated_at = NOW()
WHERE status IN ('pending_verification', 'pending_delegation')
  AND expires_at < NOW();
