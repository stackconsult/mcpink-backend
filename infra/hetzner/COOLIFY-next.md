# Coolify: Temporary Switch to `next` Branch

Switching to `next` to get the GitHub App infinite loop fix (PR #8052).

## Switch to `next` (Pinned Digest)

```bash
# Backup
cp /data/coolify/source/.env /data/coolify/source/.env.backup

# Pull and get digest
docker pull ghcr.io/coollabsio/coolify:next
docker inspect ghcr.io/coollabsio/coolify:next --format='{{index .RepoDigests 0}}'
# Save the output: ghcr.io/coollabsio/coolify@sha256:xxx

# Set pinned digest (replace sha256:xxx with actual)
sed -i '/^LATEST_IMAGE=/d' /data/coolify/source/.env
echo "LATEST_IMAGE=ghcr.io/coollabsio/coolify@sha256:xxx" >> /data/coolify/source/.env

# Restart
cd /data/coolify/source
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

## Revert to Stable

Once fix is in a stable release:

```bash
sed -i '/^LATEST_IMAGE=/d' /data/coolify/source/.env
echo "LATEST_IMAGE=4.0.0-beta.XXX" >> /data/coolify/source/.env

cd /data/coolify/source
docker compose -f docker-compose.yml -f docker-compose.prod.yml pull
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

## Track Fix Status

Check if merged to stable: https://github.com/coollabsio/coolify/releases
