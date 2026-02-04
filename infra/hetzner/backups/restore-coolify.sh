#!/bin/bash
# Coolify Restore Script
# Restores Coolify from backup files
# Usage: ./restore-coolify.sh <backup_dir> [date]
# Example: ./restore-coolify.sh /backups/coolify 20260204

set -euo pipefail

BACKUP_DIR="${1:-/backups/coolify}"
TARGET_DATE="${2:-}"
COOLIFY_DIR="/data/coolify"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1"
}

error_exit() {
    log "ERROR: $1"
    exit 1
}

confirm() {
    read -p "$1 [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        error_exit "Aborted by user"
    fi
}

# Verify backup directory exists
if [[ ! -d "$BACKUP_DIR" ]]; then
    error_exit "Backup directory not found: $BACKUP_DIR"
fi

# List available backups
log "Available backups in $BACKUP_DIR:"
echo ""
ls -la "$BACKUP_DIR" | grep -E "(config-|database-)" | head -20
echo ""

# Determine which backup to restore
if [[ -z "$TARGET_DATE" ]]; then
    # Use most recent backup
    CONFIG_BACKUP=$(ls -t "$BACKUP_DIR"/config-*.tar.gz 2>/dev/null | head -1)
    DB_BACKUP=$(ls -t "$BACKUP_DIR"/database-*.sql.gz 2>/dev/null | head -1)
else
    # Use specific date
    CONFIG_BACKUP=$(ls "$BACKUP_DIR"/config-${TARGET_DATE}*.tar.gz 2>/dev/null | head -1)
    DB_BACKUP=$(ls "$BACKUP_DIR"/database-${TARGET_DATE}*.sql.gz 2>/dev/null | head -1)
fi

if [[ -z "$CONFIG_BACKUP" ]]; then
    error_exit "No config backup found"
fi

log "Selected backups:"
log "  Config: $CONFIG_BACKUP"
log "  Database: ${DB_BACKUP:-NOT FOUND}"

echo ""
confirm "This will OVERWRITE current Coolify configuration. Continue?"

# Check if Coolify is running
COOLIFY_RUNNING=$(docker ps --filter "name=coolify" --format "{{.Names}}" | wc -l)

if [[ $COOLIFY_RUNNING -gt 0 ]]; then
    log "Stopping Coolify services..."
    cd "$COOLIFY_DIR/source"
    docker compose down || log "Warning: Could not stop Coolify gracefully"
fi

# Create restore backup of current state
RESTORE_BACKUP_DIR="/tmp/coolify-pre-restore-$(date +%Y%m%d-%H%M%S)"
log "Creating pre-restore backup at $RESTORE_BACKUP_DIR..."
mkdir -p "$RESTORE_BACKUP_DIR"
cp -r "$COOLIFY_DIR/source/.env" "$RESTORE_BACKUP_DIR/" 2>/dev/null || true
cp -r "$COOLIFY_DIR/ssh/keys" "$RESTORE_BACKUP_DIR/" 2>/dev/null || true

# Restore configuration files
log "Restoring configuration files..."
cd /
tar -xzf "$CONFIG_BACKUP" --overwrite

# Restore database if backup exists
if [[ -n "$DB_BACKUP" && -f "$DB_BACKUP" ]]; then
    log "Starting database container only..."
    cd "$COOLIFY_DIR/source"
    docker compose up -d coolify-db

    log "Waiting for database to be ready..."
    sleep 10

    # Check if database is ready
    for i in {1..30}; do
        if docker exec coolify-db pg_isready -U coolify >/dev/null 2>&1; then
            log "Database is ready"
            break
        fi
        log "Waiting for database... ($i/30)"
        sleep 2
    done

    log "Restoring database..."
    # Drop existing database and recreate
    docker exec coolify-db psql -U coolify -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='coolify' AND pid <> pg_backend_pid();" postgres 2>/dev/null || true
    docker exec coolify-db dropdb -U coolify coolify 2>/dev/null || true
    docker exec coolify-db createdb -U coolify coolify 2>/dev/null || true

    # Restore from backup
    gunzip -c "$DB_BACKUP" | docker exec -i coolify-db psql -U coolify coolify

    log "Database restore complete"
else
    log "Skipping database restore (no backup found)"
fi

# Start all Coolify services
log "Starting Coolify services..."
cd "$COOLIFY_DIR/source"
docker compose up -d

# Wait for Coolify to be healthy
log "Waiting for Coolify to be healthy..."
for i in {1..30}; do
    if curl -s http://localhost:8000/api/v1/health >/dev/null 2>&1; then
        log "Coolify is healthy!"
        break
    fi
    log "Waiting for Coolify... ($i/30)"
    sleep 5
done

# Verify services
log "Verifying services..."
docker compose ps

echo ""
log "========================================="
log "Restore complete!"
log "========================================="
log ""
log "Pre-restore backup saved to: $RESTORE_BACKUP_DIR"
log ""
log "Next steps:"
log "1. Access Coolify dashboard and verify configuration"
log "2. Check that all servers are reachable"
log "3. Test a deployment"
log "4. If issues occur, restore from: $RESTORE_BACKUP_DIR"
