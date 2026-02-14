-- name: CreateService :one
INSERT INTO services (
    id, user_id, project_id, repo, branch, server_uuid, name, build_pack, port, env_vars, git_provider, build_config, memory, vcpus, cluster_id
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
)
RETURNING *;

-- name: GetServiceByID :one
SELECT * FROM services WHERE id = $1 AND is_deleted = false;

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

-- name: SetCurrentDeploymentID :exec
UPDATE services
SET current_deployment_id = $2, updated_at = NOW()
WHERE id = $1 AND is_deleted = false;

-- name: SetServiceFQDN :exec
UPDATE services
SET fqdn = $2, updated_at = NOW()
WHERE id = $1 AND is_deleted = false;
