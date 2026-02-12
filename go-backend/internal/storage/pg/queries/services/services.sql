-- name: CreateService :one
INSERT INTO services (
    id, user_id, project_id, repo, branch, server_uuid, name, build_pack, port, env_vars, workflow_id, workflow_run_id, build_status, git_provider, build_config, memory, cpu
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 'queued', $13, $14, $15, $16
)
RETURNING *;

-- name: GetServiceByID :one
SELECT * FROM services WHERE id = $1 AND is_deleted = false;

-- name: GetServiceByWorkflowID :one
SELECT * FROM services WHERE workflow_id = $1 AND is_deleted = false;

-- name: ListServicesByUserID :many
SELECT * FROM services
WHERE user_id = $1 AND is_deleted = false
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListServicesByProjectID :many
SELECT * FROM services
WHERE project_id = $1 AND is_deleted = false
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountServicesByUserID :one
SELECT COUNT(*) FROM services WHERE user_id = $1 AND is_deleted = false;

-- name: CountServicesByProjectID :one
SELECT COUNT(*) FROM services WHERE project_id = $1 AND is_deleted = false;

-- name: UpdateBuildStatus :one
UPDATE services
SET build_status = $2, updated_at = NOW()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: UpdateRuntimeStatus :one
UPDATE services
SET runtime_status = $2, updated_at = NOW()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: UpdateServiceRunning :one
UPDATE services
SET build_status = 'success', runtime_status = 'running', fqdn = $2, commit_hash = $3, updated_at = NOW()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: UpdateServiceFailed :one
UPDATE services
SET build_status = 'failed', error_message = $2, updated_at = NOW()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: UpdateWorkflowRunID :exec
UPDATE services
SET workflow_run_id = $2, updated_at = NOW()
WHERE id = $1 AND is_deleted = false;

-- name: DeleteService :exec
DELETE FROM services WHERE id = $1;

-- name: SoftDeleteService :one
UPDATE services
SET is_deleted = true, updated_at = NOW()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: GetServicesByRepoBranch :many
SELECT * FROM services
WHERE repo = $1 AND branch = $2 AND is_deleted = false;

-- name: UpdateServiceRedeploying :one
UPDATE services
SET build_status = 'building', updated_at = NOW()
WHERE id = $1 AND is_deleted = false
RETURNING *;

-- name: GetServiceByNameAndProject :one
SELECT * FROM services
WHERE name = $1 AND project_id = $2 AND is_deleted = false;

-- name: GetServiceByNameAndUserProject :one
SELECT a.* FROM services a
JOIN projects p ON a.project_id = p.id
WHERE a.name = $1
  AND p.user_id = $2
  AND (p.ref = $3 OR ($3 = 'default' AND p.is_default = true))
  AND a.is_deleted = false
ORDER BY a.updated_at DESC, a.created_at DESC, a.id DESC
LIMIT 1;

-- name: GetServicesByRepoBranchProvider :many
SELECT * FROM services
WHERE repo = $1 AND branch = $2 AND git_provider = $3 AND is_deleted = false;

-- name: UpdateServiceBuildProgress :exec
UPDATE services
SET build_progress = $2, updated_at = NOW()
WHERE id = $1 AND is_deleted = false;

-- name: ClearServiceBuildProgress :exec
UPDATE services
SET build_progress = NULL, updated_at = NOW()
WHERE id = $1 AND is_deleted = false;
