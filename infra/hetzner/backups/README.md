# Coolify Backup Scripts

Scripts for backing up and restoring Coolify on the Factory server.

## Quick Start

### Deploy to Factory Server

```bash
# Copy scripts to Factory
scp backup-coolify.sh restore-coolify.sh root@46.225.65.56:/root/

# SSH to Factory and set up
ssh root@46.225.65.56

# Make executable
chmod +x /root/backup-coolify.sh /root/restore-coolify.sh

# Create backup directory
mkdir -p /backups/coolify

# Run first backup
/root/backup-coolify.sh

# Set up daily cron (runs at 3 AM)
echo "0 3 * * * /root/backup-coolify.sh >> /var/log/coolify-backup.log 2>&1" | crontab -
```

## Scripts

### backup-coolify.sh

Creates backups of:
- Coolify configuration files (`.env`, docker-compose files)
- SSH keys for server connections
- PostgreSQL database dump

**Usage:**
```bash
./backup-coolify.sh [backup_directory]

# Examples
./backup-coolify.sh                    # Uses default /backups/coolify
./backup-coolify.sh /mnt/external/     # Custom directory
```

**Output:**
- `config-YYYYMMDD-HHMMSS.tar.gz` - Configuration and SSH keys
- `database-YYYYMMDD-HHMMSS.sql.gz` - PostgreSQL dump
- `volumes-YYYYMMDD-HHMMSS.txt` - Docker volumes list

**Retention:** 7 days (configurable in script)

### restore-coolify.sh

Restores Coolify from backup files.

**Usage:**
```bash
./restore-coolify.sh <backup_directory> [date]

# Examples
./restore-coolify.sh /backups/coolify           # Restore latest
./restore-coolify.sh /backups/coolify 20260204  # Restore specific date
```

**What it does:**
1. Lists available backups
2. Stops Coolify services
3. Creates pre-restore backup (just in case)
4. Restores configuration files
5. Restores database
6. Starts Coolify services
7. Verifies health

## Backup Contents

| File | Contents | Restore Priority |
|------|----------|------------------|
| `config-*.tar.gz` | `.env`, docker-compose, SSH keys | Critical |
| `database-*.sql.gz` | All Coolify data (servers, apps, settings) | Critical |
| `volumes-*.txt` | Docker volumes list (reference only) | Info only |

## Disaster Recovery Scenarios

### Scenario 1: Fresh Server Install

```bash
# 1. Install Coolify
curl -fsSL https://cdn.coollabs.io/coolify/install.sh | bash

# 2. Stop Coolify
cd /data/coolify/source && docker compose down

# 3. Copy backups from offsite storage
scp user@backup-server:/backups/coolify/* /backups/coolify/

# 4. Run restore
/root/restore-coolify.sh /backups/coolify

# 5. Apply custom fork (if using)
sed -i 's|image:.*coolify:.*|image: "augustinast/coolify-fork:runtime-fix"|' \
  /data/coolify/source/docker-compose.prod.yml
docker compose pull && docker compose up -d
```

### Scenario 2: Database Corruption Only

```bash
# Restore just the database
./restore-coolify.sh /backups/coolify
```

### Scenario 3: Config Lost, Database OK

```bash
# Extract just config from backup
cd /
tar -xzf /backups/coolify/config-LATEST.tar.gz
cd /data/coolify/source && docker compose restart
```

## Offsite Backup (Recommended)

After local backup, sync to offsite storage:

```bash
# Using restic
export RESTIC_REPOSITORY="s3:https://fsn1.your-objectstorage.com/coolify-backups"
export RESTIC_PASSWORD="your-encryption-password"
restic backup /backups/coolify
restic forget --keep-daily 7 --keep-weekly 4 --prune

# Or using rclone
rclone sync /backups/coolify remote:coolify-backups/
```

## Monitoring Backup Health

Add to your monitoring:

```bash
# Check backup age (alert if > 25 hours)
LATEST=$(ls -t /backups/coolify/config-*.tar.gz | head -1)
AGE_HOURS=$(( ($(date +%s) - $(stat -c %Y "$LATEST")) / 3600 ))
if [ $AGE_HOURS -gt 25 ]; then
    echo "WARNING: Backup is $AGE_HOURS hours old"
fi
```

## Verification

After restoring, verify:

1. Dashboard loads: `https://factory.yourdomain.com`
2. All servers reachable in Coolify
3. Can trigger a deployment
4. SSH keys work: servers don't show as disconnected
