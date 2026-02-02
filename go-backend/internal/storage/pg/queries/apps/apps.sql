-- name: CreateApp :one
INSERT INTO apps (
    id, user_id, project_id, repo, branch, server_uuid, name, build_pack, port, env_vars, workflow_id, workflow_run_id, build_status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 'queued'
)
RETURNING *;

-- name: GetAppByID :one
SELECT * FROM apps WHERE id = $1 AND is_deleted = false;

-- name: GetAppByWorkflowID :one
SELECT * FROM apps WHERE workflow_id = $1 AND is_deleted = false;

-- name: GetAppByCoolifyUUID :one
SELECT * FROM apps WHERE coolify_app_uuid = $1 AND is_deleted = false;

-- name: ListAppsByUserID :many
SELECT * FROM apps
WHERE user_id = $1 AND is_deleted = false
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListAppsByProjectID :many
SELECT * FROM apps
WHERE project_id = $1 AND is_deleted = false
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountAppsByUserID :one
SELECT COUNT(*) FROM apps WHERE user_id = $1 AND is_deleted = false;

-- name: CountAppsByProjectID :one
SELECT COUNT(*) FROM apps WHERE project_id = $1 AND is_deleted = false;

-- name: UpdateBuildStatus :one
UPDATE apps
SET build_status = $2, updated_at = NOW()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: UpdateRuntimeStatus :one
UPDATE apps
SET runtime_status = $2, updated_at = NOW()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: UpdateAppCoolifyUUID :one
UPDATE apps
SET coolify_app_uuid = $2, build_status = 'building', updated_at = NOW()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: UpdateAppRunning :one
UPDATE apps
SET build_status = 'success', runtime_status = 'running', fqdn = $2, commit_hash = $3, updated_at = NOW()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: UpdateAppFailed :one
UPDATE apps
SET build_status = 'failed', error_message = $2, updated_at = NOW()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: UpdateWorkflowRunID :exec
UPDATE apps
SET workflow_run_id = $2, updated_at = NOW()
WHERE id = $1 AND is_deleted = false;

-- name: DeleteApp :exec
DELETE FROM apps WHERE id = $1;

-- name: SoftDeleteApp :one
UPDATE apps
SET is_deleted = true, updated_at = NOW()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: GetAppsByRepoBranch :many
SELECT * FROM apps
WHERE repo = $1 AND branch = $2 AND coolify_app_uuid IS NOT NULL AND is_deleted = false;

-- name: UpdateAppRedeploying :one
UPDATE apps
SET build_status = 'building', updated_at = NOW()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: GetAppByNameAndProject :one
SELECT * FROM apps
WHERE name = $1 AND project_id = $2 AND coolify_app_uuid IS NOT NULL AND is_deleted = false;

-- name: GetAppByNameAndUserProject :one
SELECT a.* FROM apps a
JOIN projects p ON a.project_id = p.id
WHERE a.name = $1
  AND p.user_id = $2
  AND (p.ref = $3 OR ($3 = 'default' AND p.is_default = true))
  AND a.is_deleted = false
LIMIT 1;
