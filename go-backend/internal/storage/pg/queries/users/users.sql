-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByGitHubID :one
SELECT * FROM users WHERE github_id = $1;

-- name: CreateUser :one
INSERT INTO users (id, github_id, github_username, github_token, avatar_url, github_scopes)
VALUES (gen_random_uuid()::TEXT, $1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateGitHubToken :one
UPDATE users
SET github_token = $2, github_username = $3, avatar_url = $4, github_scopes = $5, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteUser :exec
DELETE FROM users WHERE id = $1;

-- name: SetGitHubAppInstallation :one
UPDATE users
SET github_app_installation_id = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: ClearGitHubAppInstallation :one
UPDATE users
SET github_app_installation_id = NULL, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateGitHubScopes :one
UPDATE users
SET github_scopes = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: SetCoolifyGitHubAppUUID :one
UPDATE users
SET coolify_github_app_uuid = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;
