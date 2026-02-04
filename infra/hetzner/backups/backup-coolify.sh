#!/bin/bash
# Coolify Backup Script
# Backs up critical Coolify data to local directory
# Recommended: Run daily via cron
# Usage: ./backup-coolify.sh [backup_dir]

set -euo pipefail

# Configuration
BACKUP_DIR="${1:-/backups/coolify}"
DATE=$(date +%Y%m%d-%H%M%S)
RETENTION_DAYS=7
COOLIFY_DIR="/data/coolify"
LOG_FILE="/var/log/coolify-backup.log"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

error_exit() {
    log "ERROR: $1"
    exit 1
}

# Verify Coolify installation exists
if [[ ! -d "$COOLIFY_DIR" ]]; then
    error_exit "Coolify directory not found at $COOLIFY_DIR"
fi

# Create backup directory
mkdir -p "$BACKUP_DIR" || error_exit "Cannot create backup directory"

log "Starting Coolify backup..."
log "Backup directory: $BACKUP_DIR"

# 1. Backup configuration files
log "Backing up configuration files..."
CONFIG_BACKUP="$BACKUP_DIR/config-$DATE.tar.gz"
tar -czf "$CONFIG_BACKUP" \
    "$COOLIFY_DIR/source/.env" \
    "$COOLIFY_DIR/source/docker-compose.yml" \
    "$COOLIFY_DIR/source/docker-compose.prod.yml" \
    "$COOLIFY_DIR/ssh/keys" \
    2>/dev/null || log "Warning: Some config files may be missing"

if [[ -f "$CONFIG_BACKUP" ]]; then
    log "Config backup created: $(ls -lh "$CONFIG_BACKUP" | awk '{print $5}')"
else
    error_exit "Failed to create config backup"
fi

# 2. Backup PostgreSQL database
log "Backing up PostgreSQL database..."
DB_BACKUP="$BACKUP_DIR/database-$DATE.sql"

# Find the coolify-db container
DB_CONTAINER=$(docker ps --filter "name=coolify-db" --format "{{.Names}}" | head -1)

if [[ -z "$DB_CONTAINER" ]]; then
    log "Warning: coolify-db container not found, trying alternate names..."
    DB_CONTAINER=$(docker ps --filter "name=postgres" --format "{{.Names}}" | grep -i coolify | head -1)
fi

if [[ -n "$DB_CONTAINER" ]]; then
    if docker exec "$DB_CONTAINER" pg_dump -U coolify coolify > "$DB_BACKUP" 2>/dev/null; then
        gzip "$DB_BACKUP"
        log "Database backup created: $(ls -lh "${DB_BACKUP}.gz" | awk '{print $5}')"
    else
        log "Warning: Database backup failed - container may be unhealthy"
    fi
else
    log "Warning: Could not find Coolify database container"
fi

# 3. Backup volumes list (for reference)
log "Documenting Docker volumes..."
VOLUMES_BACKUP="$BACKUP_DIR/volumes-$DATE.txt"
docker volume ls > "$VOLUMES_BACKUP" 2>/dev/null || true

# 4. Cleanup old backups
log "Cleaning up backups older than $RETENTION_DAYS days..."
DELETED_COUNT=$(find "$BACKUP_DIR" -type f -mtime +$RETENTION_DAYS -delete -print | wc -l)
log "Deleted $DELETED_COUNT old backup files"

# 5. Verify backup integrity
log "Verifying backup integrity..."
if tar -tzf "$CONFIG_BACKUP" >/dev/null 2>&1; then
    log "Config backup integrity: OK"
else
    log "Warning: Config backup may be corrupted"
fi

if [[ -f "${DB_BACKUP}.gz" ]]; then
    if gzip -t "${DB_BACKUP}.gz" 2>/dev/null; then
        log "Database backup integrity: OK"
    else
        log "Warning: Database backup may be corrupted"
    fi
fi

# Summary
log "Backup complete!"
log "Files in backup directory:"
ls -lh "$BACKUP_DIR" | tail -10 | while read line; do log "  $line"; done

# Calculate total backup size
TOTAL_SIZE=$(du -sh "$BACKUP_DIR" | cut -f1)
log "Total backup size: $TOTAL_SIZE"

echo ""
echo "=== Backup Summary ==="
echo "Config backup: $CONFIG_BACKUP"
echo "Database backup: ${DB_BACKUP}.gz"
echo "Retention: $RETENTION_DAYS days"
echo "Total size: $TOTAL_SIZE"
