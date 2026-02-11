run-server:
	@go run cmd/server/main.go

run-worker:
	@go run cmd/worker/main.go

run-deployer-server:
	@go run cmd/deployer-server/main.go

run-deployer-worker:
	@go run cmd/deployer-worker/main.go

sqlc:
	@go run github.com/sqlc-dev/sqlc/cmd/sqlc@latest generate

gqlgen:
	@go run github.com/99designs/gqlgen@latest generate

test:
	@go test ./...

lint:
	@golangci-lint run --timeout=10m ./...
