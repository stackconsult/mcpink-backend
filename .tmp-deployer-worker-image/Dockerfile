FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY deployer-worker /app/deployer-worker
COPY application.yaml /app/application.yaml
ENTRYPOINT ["/app/deployer-worker"]
