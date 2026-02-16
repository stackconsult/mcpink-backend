-- name: CreateDeployment :one
INSERT INTO deployments (
    id, service_id, workflow_id, build_pack, build_config, env_vars_snapshot,
    memory, vcpus, port, trigger, trigger_ref, commit_hash
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING *;

-- name: GetDeploymentByID :one
SELECT * FROM deployments WHERE id = $1;

-- name: GetDeploymentByWorkflowID :one
SELECT * FROM deployments WHERE workflow_id = $1;

-- name: ListDeploymentsByServiceID :many
SELECT * FROM deployments
WHERE service_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: GetActiveDeploymentByServiceID :one
SELECT * FROM deployments
WHERE service_id = $1 AND status = 'active';

-- name: GetLatestDeploymentByServiceID :one
SELECT * FROM deployments
WHERE service_id = $1
ORDER BY created_at DESC
LIMIT 1;

-- name: UpdateDeploymentBuilding :exec
UPDATE deployments
SET status = 'building', started_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: UpdateDeploymentDeploying :exec
UPDATE deployments
SET status = 'deploying', updated_at = NOW()
WHERE id = $1;

-- name: MarkDeploymentActive :exec
UPDATE deployments
SET status = 'active', commit_hash = $2, image_ref = $3, finished_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: MarkDeploymentFailed :exec
UPDATE deployments
SET status = 'failed', error_message = $2, finished_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: MarkDeploymentCancelled :exec
UPDATE deployments
SET status = 'cancelled', finished_at = NOW(), updated_at = NOW()
WHERE id = $1;

-- name: SupersedeActiveDeployment :exec
UPDATE deployments
SET status = 'superseded', finished_at = NOW(), updated_at = NOW()
WHERE service_id = $1 AND status = 'active';

-- name: CancelInFlightDeployments :many
UPDATE deployments
SET status = 'cancelled', finished_at = NOW(), updated_at = NOW()
WHERE service_id = $1 AND status IN ('queued', 'building', 'deploying') AND id != $2
RETURNING workflow_id;

-- name: UpdateDeploymentBuildProgress :exec
UPDATE deployments
SET build_progress = $2, updated_at = NOW()
WHERE id = $1;

-- name: UpdateDeploymentCommitHash :exec
UPDATE deployments
SET commit_hash = $2, updated_at = NOW()
WHERE id = $1;

-- name: UpdateDeploymentWorkflowRunID :exec
UPDATE deployments
SET workflow_run_id = $2, updated_at = NOW()
WHERE id = $1;

-- name: CountDeploymentsByServiceID :one
SELECT COUNT(*) FROM deployments WHERE service_id = $1;

-- name: GetLatestDeploymentsByServiceIDs :many
SELECT DISTINCT ON (service_id) * FROM deployments
WHERE service_id = ANY($1::text[])
ORDER BY service_id, created_at DESC;
