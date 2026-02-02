# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go.mod and go.sum first for better layer caching
COPY go.mod go.sum ./

RUN go mod download

# Copy source code
COPY . .

# Build the worker binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o worker cmd/worker/main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata curl bash

# Install flyctl CLI
RUN curl -L https://fly.io/install.sh | bash
ENV PATH="/root/.fly/bin:$PATH"

WORKDIR /app

COPY --from=builder /app/worker .
COPY --from=builder /app/application.yaml .

CMD ["./worker"]
