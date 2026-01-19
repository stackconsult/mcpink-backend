-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByGitHubID :one
SELECT * FROM users WHERE github_id = $1;

-- name: CreateUser :one
INSERT INTO users (github_id, github_username, github_token, avatar_url)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateGitHubToken :one
UPDATE users
SET github_token = $2, github_username = $3, avatar_url = $4, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateFlyioCredentials :one
UPDATE users
SET flyio_token = $2, flyio_org = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;
