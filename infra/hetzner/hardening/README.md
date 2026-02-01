# Muscle Server Security: Practical Hardening for Launch

Secure the run servers (Muscle) for executing untrusted user code. Focus: ship a secure product, not theoretical perfection.

---

## Adding a New Muscle Server

When provisioning a new Muscle server, complete this checklist:

### Prerequisites
- [ ] Server provisioned in Hetzner
- [ ] SSH access configured
- [ ] Docker installed
- [ ] Server registered in Coolify as a destination

### One-Command Setup (Recommended)

```bash
# From the infra/hetzner/hardening directory:
./setup-muscle.sh <server-ip>
```

This applies baseline security (egress rules, gVisor available, miner detection) without making gVisor the default runtime.

### Manual Hardening Steps

If you prefer step-by-step:

```bash
# Set the new server IP
export MUSCLE_IP="<new-server-ip>"

# 1. Copy all hardening scripts
scp setup-egress-rules.sh install-gvisor.sh detect-miners.sh verify-hardening.sh root@${MUSCLE_IP}:/root/
ssh root@${MUSCLE_IP} "chmod +x /root/*.sh"

# 2. Apply egress firewall rules
ssh root@${MUSCLE_IP} "bash /root/setup-egress-rules.sh"

# 3. Install gVisor
ssh root@${MUSCLE_IP} "bash /root/install-gvisor.sh"

# 4. Configure Docker daemon (baseline - gVisor available but not default)
scp daemon-baseline.json root@${MUSCLE_IP}:/etc/docker/daemon.json
ssh root@${MUSCLE_IP} "systemctl restart docker"

# 5. Set up miner detection cron (runs every 5 min)
ssh root@${MUSCLE_IP} 'echo "*/5 * * * * /root/detect-miners.sh" | crontab -'

# 6. Verify setup
ssh root@${MUSCLE_IP} "bash /root/verify-hardening.sh"
```

### Testing gVisor

After baseline setup, test gVisor before making it default:

```bash
# Basic test
ssh root@${MUSCLE_IP} "docker run --runtime=runsc --rm hello-world"

# Alpine test
ssh root@${MUSCLE_IP} "docker run --runtime=runsc --rm alpine echo 'gVisor works'"

# Node.js test
ssh root@${MUSCLE_IP} "docker run --runtime=runsc --rm node:20-alpine node -e 'console.log(process.version)'"
```

### Making gVisor the Default Runtime

Once testing is complete:

```bash
# Switch to gVisor as default
scp daemon.json root@${MUSCLE_IP}:/etc/docker/daemon.json
ssh root@${MUSCLE_IP} "systemctl restart docker"

# Verify system containers still running
ssh root@${MUSCLE_IP} "docker ps | grep -E 'traefik|coolify'"
```

### Verification Checklist
- [ ] `iptables -L DOCKER-USER -n` shows DROP rules for metadata, SMTP, mining ports
- [ ] `docker run --runtime=runsc --rm hello-world` succeeds
- [ ] SMTP test times out or refuses (not "open")
- [ ] Traefik still running: `docker ps | grep traefik`
- [ ] `bash /root/verify-hardening.sh` passes all critical checks

---

## Threat Model: What You're Actually Defending Against

| Threat | Likelihood | Impact | Mitigation |
|--------|------------|--------|------------|
| Crypto mining | High | High (CPU/cost) | CPU limits + process monitoring |
| Spam/phishing | High | High (IP reputation) | Block SMTP ports |
| DDoS launch point | Medium | High (IP blacklist) | Rate limits + egress rules |
| Container escape | Low | Critical | gVisor |
| Credential theft (metadata) | Medium | Critical | Block 169.254.169.254 |
| Fork bomb / resource exhaustion | Medium | Medium | pids-limit, memory limits |
| Disk fill attack | Medium | Medium | Ephemeral storage limits |

---

## What Apps Should NOT Run (and How to Block)

### Explicitly Block at Deployment Time

| Blocked | Detection | Action |
|---------|-----------|--------|
| Crypto miners | Dockerfile has xmrig, cgminer, etc. | Reject at build |
| Tor exit nodes | Port 9001, 9030 in start command | Reject |
| VPN/proxy servers | OpenVPN, Wireguard, Shadowsocks | Reject |
| Torrenting | qBittorrent, Transmission | Reject |
| Mail servers | Postfix, Sendmail, SMTP apps | Reject |

**Practical implementation**: Scan Dockerfile and start commands for keywords before deploying. Reject with clear error message.

```go
// InkMCP validation
blockedPatterns := []string{
    "xmrig", "cgminer", "nicehash", "ethminer",
    "openvpn", "wireguard", "shadowsocks",
    "qbittorrent", "transmission-daemon",
    "postfix", "sendmail", "exim",
}
```

### Block at Runtime (Egress Rules)

See `setup-egress-rules.sh` - blocks:
- Cloud metadata endpoints (credential theft)
- SMTP ports (spam prevention)
- Common mining pool ports
- IRC ports (botnet C&C)
- Tor directory/relay ports

### Runtime Monitoring (Detect & Kill)

See `detect-miners.sh` - detects containers with sustained high CPU usage.

---

## Container Resource Limits (Enforce in InkMCP)

Every user container MUST have:

```bash
docker run -d \
  --memory=512m \          # Hard limit - OOM kills if exceeded
  --memory-swap=512m \     # No swap (prevents disk thrashing)
  --cpus=0.5 \             # CPU quota (supports fractions: 0.25, 0.5, 1, 2)
  --pids-limit=256 \       # Prevent fork bombs
  --cap-drop=ALL \         # No capabilities
  --security-opt=no-new-privileges \
  --read-only \            # Read-only root filesystem
  --tmpfs /tmp:size=512m,mode=1777 \  # Writable /tmp (RAM-backed)
  user-app:latest
```

### Resource Tiers

| Tier | Memory | CPU | Ephemeral Disk | Use Case |
|------|--------|-----|----------------|----------|
| Free | 256m | 0.25 | 512MB (tmpfs) | Simple MCP servers |
| Basic | 512m | 0.5 | 1GB (tmpfs) | Most apps |
| Pro | 1g | 1.0 | 2GB (tmpfs) | Heavier workloads |
| Custom | 2g+ | 2.0+ | 5GB+ | PDF processing, ML, etc. |

### Ephemeral Storage Options

**Option 1: tmpfs (RAM-backed)** - Default, fast, uses container memory allocation
```bash
--tmpfs /tmp:size=512m,mode=1777
--tmpfs /app/data:size=1g
```

**Option 2: Disk-backed ephemeral volume** - For larger storage needs (5GB+), doesn't use RAM
```bash
# Requires overlay2 with xfs and project quotas on host
--storage-opt size=10G
```

**Trade-offs:**
| Type | Speed | Size Limit | Uses RAM | Survives Restart |
|------|-------|------------|----------|------------------|
| tmpfs | Fast | Limited by RAM | Yes | No |
| storage-opt | Disk speed | 10GB+ possible | No | No |

For most MCP servers, tmpfs at 512MB-2GB is sufficient. If users need to process large files (PDFs, images, ML models), either:
1. Stream directly to object storage (R2/S3)
2. Upgrade to a tier with disk-backed ephemeral storage

**Note**: Coolify may set some of these. Verify Coolify's compose output includes these constraints.

---

## Docker Daemon Configuration

### With gVisor (Production)

Use `daemon.json`:

```json
{
  "metrics-addr": "127.0.0.1:9323",
  "no-new-privileges": true,
  "live-restore": true,
  "userland-proxy": false,
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  },
  "default-runtime": "runsc",
  "runtimes": {
    "runsc": {
      "path": "/usr/local/bin/runsc"
    },
    "runc": {
      "path": "/usr/bin/runc"
    }
  }
}
```

### Without gVisor (Baseline/Testing)

Use `daemon-baseline.json` if gVisor isn't ready yet.

---

## System Containers (Must Use runc)

These containers MUST run on runc (not gVisor):

| Container | Why runc |
|-----------|----------|
| traefik | Coolify proxy - needs full network access |
| coolify | Coolify core services |
| cadvisor | Needs /sys, /proc access for metrics |
| alloy | Grafana agent - same reason |
| Any container with `coolify` in name | Coolify internal |

Since we set `default-runtime: runsc`, these system containers need explicit `--runtime=runc` in their compose/run commands. Coolify handles its own containers, but verify after enabling gVisor.

---

## What gVisor Protects Against

| Scenario | Without gVisor | With gVisor |
|----------|----------------|-------------|
| Kernel exploit (CVE-2024-21626) | Full host access | Blocked - hits gVisor kernel |
| Container escape via runc bug | Host compromise | Blocked |
| /proc, /sys snooping | Can read host info | Sandboxed |
| ptrace attacks | Possible | Blocked |

**What gVisor does NOT protect against**:
- Network abuse (that's why you need egress rules)
- Resource exhaustion (that's why you need limits)
- Application-level vulns (your problem)

---

## Compatibility: What Breaks Under gVisor

| Works | Doesn't Work |
|-------|--------------|
| Node.js, Python, Go, Rust, Java | Raw sockets (some network tools) |
| PostgreSQL, Redis, MongoDB | io_uring (some high-perf libs) |
| Next.js, Remix, FastAPI | strace, gdb, perf |
| WebSockets, HTTP/2 | Some kernel-specific syscalls |

**Practical impact**: 95%+ of web apps work. If something breaks, user can report and you can investigate.

---

## Files in This Directory

| File | Purpose |
|------|---------|
| `README.md` | This documentation |
| `setup-muscle.sh` | **One-command setup** for new Muscle servers |
| `setup-egress-rules.sh` | Egress firewall rules (metadata, SMTP, mining, IRC, Tor) |
| `install-gvisor.sh` | gVisor installation |
| `detect-miners.sh` | Runtime abuse detection (cron) |
| `verify-hardening.sh` | Automated security verification |
| `harden-ssh.sh` | SSH hardening (disable password auth) |
| `setup-host-firewall.sh` | UFW host firewall configuration |
| `daemon.json` | Docker daemon config with gVisor as default |
| `daemon-baseline.json` | Docker daemon config with gVisor available (not default) |

---

## Monitoring for Abuse

Add to Grafana Cloud alerts:

```promql
# High sustained CPU (potential miner)
avg_over_time(container_cpu_usage_seconds_total{container_name!~"traefik|cadvisor"}[5m]) > 0.9

# Unusual network egress
rate(container_network_transmit_bytes_total[5m]) > 10000000  # 10MB/s
```

---

## Troubleshooting

| Problem | Fix |
|---------|-----|
| App doesn't work under gVisor | Run with `--runtime=runc` as temporary workaround, investigate |
| Traefik down after gVisor | Ensure Traefik uses runc: `docker update --runtime=runc traefik` |
| Can't ssh after iptables | Hetzner rescue mode, or use Robot console |
| User reports app is slow | gVisor has ~5-15% overhead, acceptable for security |
| New server not receiving traffic | Check Coolify destination config, verify Traefik labels |
