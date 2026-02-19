-- name: CreateToken :one
INSERT INTO git_tokens (id, token_hash, token_prefix, user_id, repo_id, scopes, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetTokenByHash :one
SELECT gt.*, ir.full_name AS repo_full_name
FROM git_tokens gt
LEFT JOIN internal_repos ir ON gt.repo_id = ir.id
WHERE gt.token_hash = $1
  AND gt.revoked_at IS NULL
  AND (gt.expires_at IS NULL OR gt.expires_at > NOW());

-- name: ListByRepoID :many
SELECT * FROM git_tokens
WHERE repo_id = $1
  AND revoked_at IS NULL
ORDER BY created_at DESC;

-- name: ListByUserID :many
SELECT * FROM git_tokens
WHERE user_id = $1
  AND revoked_at IS NULL
ORDER BY created_at DESC;

-- name: RevokeToken :exec
UPDATE git_tokens SET revoked_at = NOW() WHERE id = $1;

-- name: RevokeTokensByRepoID :exec
UPDATE git_tokens SET revoked_at = NOW()
WHERE repo_id = $1 AND revoked_at IS NULL;

-- name: UpdateLastUsed :exec
UPDATE git_tokens SET last_used_at = NOW() WHERE id = $1;

-- name: CleanupExpired :exec
DELETE FROM git_tokens WHERE expires_at < NOW() - INTERVAL '7 days';
