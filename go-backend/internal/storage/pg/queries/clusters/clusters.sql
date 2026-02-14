-- name: GetClusterByID :one
SELECT * FROM clusters WHERE id = $1;

-- name: ListActiveClusters :many
SELECT * FROM clusters WHERE status = 'active' ORDER BY name;

-- name: GetClusterByServiceID :one
SELECT c.* FROM clusters c
JOIN services s ON s.cluster_id = c.id
WHERE s.id = $1 AND s.is_deleted = false;
