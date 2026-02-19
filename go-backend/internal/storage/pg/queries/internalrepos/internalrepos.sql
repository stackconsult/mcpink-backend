-- name: CreateInternalRepo :one
INSERT INTO internal_repos (id, user_id, project_id, name, clone_url, provider, repo_id, full_name, bare_path)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: GetInternalRepoByID :one
SELECT * FROM internal_repos WHERE id = $1;

-- name: GetInternalRepoByFullName :one
SELECT * FROM internal_repos WHERE full_name = $1;

-- name: GetInternalRepoByProjectAndName :one
SELECT * FROM internal_repos WHERE project_id = $1 AND name = $2;

-- name: ListInternalReposByUserID :many
SELECT * FROM internal_repos
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: DeleteInternalRepo :exec
DELETE FROM internal_repos WHERE id = $1;

-- name: DeleteInternalRepoByFullName :exec
DELETE FROM internal_repos WHERE full_name = $1;
