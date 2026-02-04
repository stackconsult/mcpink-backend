# Disaster Recovery Guide

This document outlines failure scenarios, recovery procedures, and backup strategies for the Hetzner infrastructure.

---

## Architecture Recap

```
InkMCP API (Plane A)
    |
    v
Factory (Plane B) - Coolify Master
    |
    +--SSH--> Muscle-1 (Plane C) - User containers
    +--SSH--> Muscle-Ops-1 (Plane C) - Registry, Gitea, Monitoring
    +--SSH--> Builder-1 - Build server
```

---

## Critical Insight: Application Independence

**Key Point**: Deployed applications continue running even if Coolify goes down.

Coolify saves all configurations (Docker Compose files, environment variables) directly on the target servers. Applications are standard Docker containers managed by Docker Compose on each server, independent of the Coolify instance.

| Coolify Status | Running Apps | New Deployments | Dashboard |
|----------------|--------------|-----------------|-----------|
| Online         | Running      | Working         | Available |
| Offline        | Running      | Blocked         | Down      |
| Corrupted      | Running      | Blocked         | Down      |

---

## Failure Scenarios

### Scenario 1: Factory Server Complete Failure

**Symptoms**: Coolify dashboard unreachable, SSH to Factory fails

**Impact**:
- Existing applications on Muscle servers: **CONTINUE RUNNING**
- New deployments: **BLOCKED**
- Dashboard access: **DOWN**
- Build jobs: **BLOCKED**

**Recovery Time Objective (RTO)**: 2-4 hours (new server + restore)

**Recovery Procedure**:
1. Provision new VPS in Hetzner Cloud (same region for vSwitch)
2. Attach to existing vSwitch/Cloud Network
3. Restore Coolify from backup (see Backup Strategy below)
4. Update DNS records if using new IP
5. Reconnect to Muscle servers (SSH keys are already on Muscle servers)

### Scenario 2: Coolify Service Corruption

**Symptoms**: Dashboard errors, deployments fail, database errors

**Impact**:
- Existing applications: **CONTINUE RUNNING**
- New deployments: **BLOCKED**
- Dashboard: **PARTIALLY WORKING** or erroring

**Recovery Procedure**:
1. SSH to Factory: `ssh root@46.225.65.56`
2. Check Coolify logs: `docker compose -f /data/coolify/source/docker-compose.yml logs -f`
3. Try restart: `docker compose -f /data/coolify/source/docker-compose.yml restart`
4. If corruption, restore from backup (see below)

### Scenario 3: Muscle Server Failure

**Symptoms**: Apps on that server unreachable, health checks failing

**Impact**:
- Applications on failed server: **DOWN**
- Applications on other servers: **UNAFFECTED**
- Coolify dashboard: **WORKING** (shows server unreachable)

**Recovery Procedure**:
1. Provision new dedicated server (or rescue existing)
2. Apply hardening: `./hardening/setup-muscle.sh <new-ip>`
3. Add SSH key from Coolify
4. Add server to Coolify
5. Redeploy affected applications

### Scenario 4: Database Corruption (Coolify PostgreSQL)

**Symptoms**: Login fails, API errors, 500 errors on dashboard

**Impact**:
- Existing applications: **CONTINUE RUNNING**
- All Coolify operations: **BLOCKED**

**Recovery Procedure**:
1. Stop Coolify: `cd /data/coolify/source && docker compose down`
2. Restore PostgreSQL data from backup
3. Start Coolify: `docker compose up -d`

### Scenario 5: Network/vSwitch Failure

**Symptoms**: Internal connectivity lost, registry pulls fail

**Impact**:
- Public-facing apps: **WORKING** (via public IPs)
- Internal registry pulls: **BLOCKED**
- Coolify SSH to servers: **MAY FAIL** (if using vSwitch IPs)

**Recovery Procedure**:
1. Verify vSwitch status in Hetzner Robot
2. Check netplan on dedicated servers: `cat /etc/netplan/50-vswitch.yaml`
3. Reapply if needed: `netplan apply`
4. Fallback to public IPs if vSwitch is down

---

## Backup Strategy

### Coolify Data Locations on Factory

| Path | Contents | Backup Priority |
|------|----------|-----------------|
| `/data/coolify/source` | Coolify installation | Low (reinstall) |
| `/data/coolify/source/.env` | Coolify config | **CRITICAL** |
| `/data/coolify/databases/coolify` | PostgreSQL data | **CRITICAL** |
| `/data/coolify/ssh/keys` | SSH keys | **CRITICAL** |
| `/data/coolify/ssh/mux` | SSH multiplexer | Low |

### Backup Commands

```bash
# Create backup directory
mkdir -p /backups/coolify

# Backup critical files
tar -czvf /backups/coolify/coolify-config-$(date +%Y%m%d).tar.gz \
  /data/coolify/source/.env \
  /data/coolify/ssh/keys

# Backup database (while Coolify is running)
docker exec coolify-db pg_dump -U coolify coolify > /backups/coolify/coolify-db-$(date +%Y%m%d).sql
```

### Automated Backup Script

Create `/root/backup-coolify.sh`:

```bash
#!/bin/bash
set -euo pipefail

BACKUP_DIR="/backups/coolify"
DATE=$(date +%Y%m%d-%H%M%S)
RETENTION_DAYS=7

mkdir -p "$BACKUP_DIR"

echo "[$(date)] Starting Coolify backup..."

# Backup config files
tar -czf "$BACKUP_DIR/config-$DATE.tar.gz" \
  /data/coolify/source/.env \
  /data/coolify/source/docker-compose.yml \
  /data/coolify/source/docker-compose.prod.yml \
  /data/coolify/ssh/keys 2>/dev/null || true

# Backup database
docker exec coolify-db pg_dump -U coolify coolify > "$BACKUP_DIR/database-$DATE.sql"
gzip "$BACKUP_DIR/database-$DATE.sql"

# Cleanup old backups
find "$BACKUP_DIR" -type f -mtime +$RETENTION_DAYS -delete

echo "[$(date)] Backup complete: $BACKUP_DIR"
ls -lh "$BACKUP_DIR"
```

Add cron job:
```bash
chmod +x /root/backup-coolify.sh
echo "0 3 * * * /root/backup-coolify.sh >> /var/log/coolify-backup.log 2>&1" | crontab -
```

### Offsite Backup (Recommended)

Use restic or rclone to sync to S3-compatible storage:

```bash
# Install restic
apt install restic

# Initialize repository (once)
export RESTIC_REPOSITORY="s3:https://fsn1.your-objectstorage.com/coolify-backups"
export RESTIC_PASSWORD="your-encryption-password"
export AWS_ACCESS_KEY_ID="your-key"
export AWS_SECRET_ACCESS_KEY="your-secret"
restic init

# Backup command
restic backup /backups/coolify

# Retention policy
restic forget --keep-daily 7 --keep-weekly 4 --keep-monthly 3 --prune
```

---

## Recovery Procedures

### Full Factory Server Recovery

**Prerequisites**: New VPS provisioned, attached to vSwitch

```bash
# 1. Install Coolify
curl -fsSL https://cdn.coollabs.io/coolify/install.sh | bash

# 2. Stop Coolify to restore data
cd /data/coolify/source
docker compose down

# 3. Restore .env file
# (copy from backup or recreate from documentation)

# 4. Restore SSH keys
mkdir -p /data/coolify/ssh/keys
# Copy from backup

# 5. Restore database
gunzip < /backups/coolify/database-YYYYMMDD.sql.gz | \
  docker exec -i coolify-db psql -U coolify coolify

# 6. If using custom Coolify fork, update docker-compose.prod.yml
sed -i 's|image:.*coolify:.*|image: "augustinast/coolify-fork:runtime-fix"|' \
  /data/coolify/source/docker-compose.prod.yml

# 7. Start Coolify
docker compose up -d

# 8. Verify
curl -s http://localhost:8000/api/v1/health
```

### Database-Only Recovery

```bash
# 1. Stop Coolify
cd /data/coolify/source
docker compose down

# 2. Remove corrupted database
rm -rf /data/coolify/databases/coolify/*

# 3. Start PostgreSQL only
docker compose up -d coolify-db

# 4. Wait for PostgreSQL to be ready
sleep 10

# 5. Restore from backup
gunzip -c /backups/coolify/database-YYYYMMDD.sql.gz | \
  docker exec -i coolify-db psql -U coolify coolify

# 6. Start remaining services
docker compose up -d
```

### SSH Key Recovery

If SSH keys are lost, you need to regenerate and redeploy:

```bash
# 1. Generate new key on Factory
ssh-keygen -t ed25519 -f /data/coolify/ssh/keys/id.root@host.docker.internal -N ""

# 2. For each Muscle server, add the new public key
ssh root@<MUSCLE_IP> "cat >> ~/.ssh/authorized_keys" < \
  /data/coolify/ssh/keys/id.root@host.docker.internal.pub

# 3. Update the key in Coolify dashboard
# Coolify Settings -> Private Keys -> Update
```

---

## Monitoring and Alerting

### Recommended Alerts

| Metric | Threshold | Action |
|--------|-----------|--------|
| Factory unreachable | > 5 min | Page on-call |
| Coolify API unhealthy | > 2 min | Page on-call |
| Muscle server unreachable | > 5 min | Alert team |
| Database connection errors | > 0 | Investigate |
| Disk usage > 85% | - | Clean up / expand |
| Backup age > 25 hours | - | Fix backup job |

### Health Check Endpoint

Monitor Coolify health:
```bash
curl -s https://factory.yourdomain.com/api/v1/health
```

### Simple Uptime Monitoring

Add these URLs to your monitoring service (Uptime Kuma, Better Uptime, etc.):

- `https://factory.yourdomain.com` - Coolify dashboard
- `https://s1.ml.ink` - Muscle-1 Traefik (any deployed app)
- `https://registry.tops.subj.org/v2/` - Docker registry

---

## Runbook: Common Issues

### Coolify Dashboard Returns 502

```bash
ssh root@46.225.65.56
cd /data/coolify/source
docker compose logs coolify -n 100
docker compose restart coolify
```

### Deployment Stuck in "Building"

```bash
# Check build server
ssh root@46.225.92.127
docker ps -a | head
journalctl -u docker -n 50

# Force-fail stuck deployment via Coolify API or dashboard
```

### Server Shows "Unreachable" in Coolify

```bash
# From Factory, test SSH
ssh -i /data/coolify/ssh/keys/id.root@host.docker.internal root@<server-ip>

# If SSH fails, check:
# 1. Server is up (Hetzner console)
# 2. SSH service running: systemctl status sshd
# 3. Firewall allows SSH: ufw status
# 4. SSH key is authorized
```

### Registry Pull Fails

```bash
# Test registry from Muscle server
docker pull registry.tops.subj.org/test:latest

# Check registry status
curl -u username:password https://registry.tops.subj.org/v2/_catalog

# Check registry logs on Muscle-Ops
ssh root@116.202.163.209
docker logs <registry-container>
```

---

## Preventive Measures

### Weekly Maintenance Checklist

- [ ] Verify backups exist and are recent: `ls -la /backups/coolify/`
- [ ] Check RAID health on dedicated servers: `cat /proc/mdstat`
- [ ] Review disk usage: `df -h`
- [ ] Check Coolify logs for warnings: `docker compose logs --since 24h | grep -i warn`
- [ ] Verify all servers reachable in Coolify dashboard
- [ ] Test a deployment to confirm pipeline works

### Monthly Tasks

- [ ] Test backup restore procedure (on staging environment)
- [ ] Review and rotate credentials if needed
- [ ] Update Coolify to latest version (after testing)
- [ ] Review security patches for OS

---

## Emergency Contacts

| Role | Contact | When to Contact |
|------|---------|-----------------|
| Hetzner Support | https://robot.hetzner.com/support | Hardware issues, network outages |
| Coolify Discord | https://discord.gg/coolify | Coolify-specific bugs |
| On-call Engineer | (internal) | Any production incident |

---

## Single Points of Failure Analysis

| Component | Risk | Mitigation |
|-----------|------|------------|
| Factory server | Coolify control plane down | Regular backups, quick restore procedure |
| Coolify database | All configuration lost | Daily database backups, offsite copies |
| SSH keys | Cannot deploy to servers | Backup keys, can regenerate |
| DNS (Cloudflare) | Apps unreachable | Cloudflare has built-in redundancy |
| vSwitch | Internal comms fail | Fallback to public IPs |
| Muscle-1 (single) | User apps down | Add Muscle-2 for redundancy |

### Recommendations for Improved Resilience

1. **Add Second Muscle Server**: Distribute apps across multiple servers
2. **Geographic Distribution**: Consider a second region for disaster recovery
3. **Automated Backup Verification**: Test restores automatically
4. **Database Replication**: Consider PostgreSQL streaming replication for Coolify DB
5. **Infrastructure as Code**: Define all servers in Terraform for quick recreation

---

## Version History

| Date | Change | Author |
|------|--------|--------|
| 2026-02-04 | Initial creation | Infrastructure Agent |
