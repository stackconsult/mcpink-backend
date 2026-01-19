-- name: CreateAPIKey :one
INSERT INTO api_keys (user_id, name, key_hash, key_prefix)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetAPIKeyByPrefix :one
SELECT * FROM api_keys
WHERE key_prefix = $1 AND revoked_at IS NULL;

-- name: ListAPIKeysByUserID :many
SELECT id, user_id, name, key_prefix, last_used_at, created_at
FROM api_keys
WHERE user_id = $1 AND revoked_at IS NULL
ORDER BY created_at DESC;

-- name: UpdateAPIKeyLastUsed :exec
UPDATE api_keys SET last_used_at = NOW() WHERE id = $1;

-- name: RevokeAPIKey :exec
UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND user_id = $2;

-- name: GetAPIKeyWithUser :one
SELECT
    ak.*,
    u.github_id,
    u.github_username,
    u.avatar_url
FROM api_keys ak
JOIN users u ON ak.user_id = u.id
WHERE ak.key_prefix = $1 AND ak.revoked_at IS NULL;
