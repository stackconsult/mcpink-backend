-- name: CreateService :one
INSERT INTO services (
    user_id, repo, branch, server_uuid, name, build_pack, port, env_vars, workflow_id, build_status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, 'queued'
)
RETURNING *;

-- name: GetServiceByID :one
SELECT * FROM services WHERE id = $1;

-- name: GetServiceByWorkflowID :one
SELECT * FROM services WHERE workflow_id = $1;

-- name: GetServiceByCoolifyUUID :one
SELECT * FROM services WHERE coolify_app_uuid = $1;

-- name: ListServicesByUserID :many
SELECT * FROM services
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountServicesByUserID :one
SELECT COUNT(*) FROM services WHERE user_id = $1;

-- name: UpdateBuildStatus :one
UPDATE services
SET build_status = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateRuntimeStatus :one
UPDATE services
SET runtime_status = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateServiceCoolifyApp :one
UPDATE services
SET coolify_app_uuid = $2, build_status = 'building', updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateServiceRunning :one
UPDATE services
SET build_status = 'success', runtime_status = 'running', fqdn = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateServiceFailed :one
UPDATE services
SET build_status = 'failed', error_message = $2, updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: UpdateWorkflowRunID :exec
UPDATE services
SET workflow_run_id = $2, updated_at = NOW()
WHERE id = $1;

-- name: DeleteService :exec
DELETE FROM services WHERE id = $1;
