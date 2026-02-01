-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByGitHubID :one
SELECT * FROM users WHERE github_id = $1;

-- name: CreateUser :one
INSERT INTO users (id, github_id, github_username, avatar_url)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateUserProfile :one
UPDATE users
SET github_username = $2, avatar_url = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: SetCoolifyGitHubAppUUID :one
UPDATE users
SET coolify_github_app_uuid = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;
