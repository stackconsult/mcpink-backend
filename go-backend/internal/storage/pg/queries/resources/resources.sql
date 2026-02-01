-- name: CreateResource :one
INSERT INTO resources (
    user_id, project_id, name, type, provider, region, external_id, credentials, metadata, status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
RETURNING *;

-- name: GetResourceByID :one
SELECT * FROM resources WHERE id = $1;

-- name: GetResourceByUserAndName :one
SELECT * FROM resources WHERE user_id = $1 AND name = $2;

-- name: ListResourcesByUser :many
SELECT * FROM resources
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListResourcesByUserAndType :many
SELECT * FROM resources
WHERE user_id = $1 AND type = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;

-- name: ListResourcesByProject :many
SELECT * FROM resources
WHERE project_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountResourcesByUser :one
SELECT COUNT(*) FROM resources WHERE user_id = $1;

-- name: UpdateResourceStatus :exec
UPDATE resources
SET status = $2, updated_at = NOW()
WHERE id = $1;

-- name: UpdateResourceCredentials :exec
UPDATE resources
SET credentials = $2, external_id = $3, metadata = $4, status = $5, updated_at = NOW()
WHERE id = $1;

-- name: UpdateResourceAfterProvisioning :one
UPDATE resources
SET external_id = $2, credentials = $3, metadata = $4, status = 'active', updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteResource :exec
DELETE FROM resources WHERE id = $1;

-- name: DeleteResourceByUserAndID :exec
DELETE FROM resources WHERE id = $1 AND user_id = $2;
