-- +goose Up
CREATE UNIQUE INDEX idx_users_gitea_username ON users(gitea_username) WHERE gitea_username IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_users_gitea_username;
