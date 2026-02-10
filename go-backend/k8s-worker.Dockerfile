# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN GOMAXPROCS=2 CGO_ENABLED=0 GOOS=linux GOFLAGS="-p=1" go build -ldflags="-s -w" -o k8s-worker cmd/k8s-worker/main.go

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata git && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/k8s-worker .
COPY --from=builder /app/application.yaml .

CMD ["./k8s-worker"]
