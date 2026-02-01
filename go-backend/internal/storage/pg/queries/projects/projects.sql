-- name: CreateProject :one
INSERT INTO projects (user_id, name, ref)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetProjectByID :one
SELECT * FROM projects WHERE id = $1;

-- name: GetProjectByRef :one
SELECT * FROM projects WHERE user_id = $1 AND ref = $2;

-- name: CreateDefaultProject :one
INSERT INTO projects (user_id, name, ref, is_default)
VALUES ($1, 'default', 'default', TRUE)
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
SET name = $2, ref = $3, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = $1;
