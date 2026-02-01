-- name: GetGitHubCredsByUserID :one
SELECT * FROM github_creds WHERE user_id = $1;

-- name: GetGitHubCredsByGitHubID :one
SELECT * FROM github_creds WHERE github_id = $1;

-- name: CreateGitHubCreds :one
INSERT INTO github_creds (user_id, github_id, github_oauth_token, github_oauth_scopes, github_app_installation_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateGitHubOAuthToken :one
UPDATE github_creds
SET github_oauth_token = $2,
    github_oauth_scopes = $3,
    github_oauth_updated_at = NOW(),
    updated_at = NOW()
WHERE user_id = $1
RETURNING *;

-- name: SetGitHubAppInstallation :one
UPDATE github_creds
SET github_app_installation_id = $2,
    updated_at = NOW()
WHERE user_id = $1
RETURNING *;

-- name: ClearGitHubAppInstallation :one
UPDATE github_creds
SET github_app_installation_id = NULL,
    updated_at = NOW()
WHERE user_id = $1
RETURNING *;

-- name: GetUserWithGitHubCreds :one
SELECT
    u.id,
    u.github_id,
    u.github_username,
    u.avatar_url,
    u.coolify_github_app_uuid,
    u.created_at,
    u.updated_at,
    gc.github_oauth_token,
    gc.github_oauth_scopes,
    gc.github_oauth_updated_at,
    gc.github_app_installation_id
FROM users u
JOIN github_creds gc ON u.id = gc.user_id
WHERE u.id = $1;
