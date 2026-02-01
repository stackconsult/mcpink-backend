-- name: CreateProject :one
INSERT INTO projects (user_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: GetProjectByID :one
SELECT * FROM projects WHERE id = $1;

-- name: GetProjectByName :one
SELECT * FROM projects WHERE user_id = $1 AND name = $2;

-- name: CreateDefaultProject :one
INSERT INTO projects (user_id, name, is_default)
VALUES ($1, 'default', TRUE)
RETURNING *;

-- name: GetDefaultProject :one
SELECT * FROM projects WHERE user_id = $1 AND is_default = TRUE;

-- name: ListProjectsByUserID :many
SELECT * FROM projects
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountProjectsByUserID :one
SELECT COUNT(*) FROM projects WHERE user_id = $1;

-- name: UpdateProjectName :one
UPDATE projects
SET name = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = $1;
