run-server:
	@go run cmd/server/main.go

sqlc:
	@go run github.com/sqlc-dev/sqlc/cmd/sqlc@latest generate

gqlgen:
	@go run github.com/99designs/gqlgen@latest generate

test:
	@go test ./...