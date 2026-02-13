# Run Node Security: Practical Hardening for Launch

Secure the run nodes for executing untrusted user code. Focus: ship a secure product, not theoretical perfection.

---

## Adding a New Run Node

When provisioning a new run node, complete this checklist:

### Prerequisites

- [ ] Server provisioned in Hetzner
- [ ] SSH key access configured and tested
- [ ] Docker installed
- [ ] Server registered in Coolify as a destination

### One-Command Setup (Recommended)

```bash
# From the infra/hetzner/hardening directory:
./setup-run-node.sh <server-ip>
```

This applies baseline security (egress rules, gVisor available, miner detection) without making gVisor the default runtime.

### Full Hardening (Including gVisor Default + SSH)

For complete hardening with gVisor as default and SSH hardening:

```bash
export RUN_NODE_IP="<server-ip>"

# Step 1: Run baseline setup
./setup-run-node.sh ${RUN_NODE_IP}

# Step 2: Apply SSH hardening
scp harden-ssh.sh root@${RUN_NODE_IP}:/root/
ssh root@${RUN_NODE_IP} "bash /root/harden-ssh.sh"

# Step 3: Make gVisor the default runtime
scp daemon.json root@${RUN_NODE_IP}:/etc/docker/daemon.json
ssh root@${RUN_NODE_IP} "systemctl restart docker"

# Step 4: Verify everything
ssh root@${RUN_NODE_IP} "bash /root/verify-hardening.sh"
```

### Manual Hardening Steps

If you prefer step-by-step control:

```bash
export RUN_NODE_IP="<new-server-ip>"

# 1. Copy all hardening scripts
scp setup-egress-rules.sh install-gvisor.sh detect-miners.sh \
    verify-hardening.sh harden-ssh.sh root@${RUN_NODE_IP}:/root/
ssh root@${RUN_NODE_IP} "chmod +x /root/*.sh"

# 2. Apply egress firewall rules (immediate, no restart needed)
ssh root@${RUN_NODE_IP} "bash /root/setup-egress-rules.sh"

# 3. Install gVisor binaries
ssh root@${RUN_NODE_IP} "bash /root/install-gvisor.sh"

# 4. Configure Docker daemon (gVisor as default)
scp daemon.json root@${RUN_NODE_IP}:/etc/docker/daemon.json
ssh root@${RUN_NODE_IP} "systemctl restart docker"

# 5. Set up miner detection cron (runs every 5 min)
ssh root@${RUN_NODE_IP} 'crontab -l 2>/dev/null | grep -v "detect-miners" | { cat; echo "*/5 * * * * /root/detect-miners.sh"; } | crontab -'

# 6. Apply SSH hardening (AFTER verifying key auth works)
ssh root@${RUN_NODE_IP} "bash /root/harden-ssh.sh"

# 7. Verify setup
ssh root@${RUN_NODE_IP} "bash /root/verify-hardening.sh"
```

---

## Testing gVisor

After setup, verify gVisor works:

```bash
# Basic test - should show gVisor's emulated kernel (4.4.0)
ssh root@${RUN_NODE_IP} "docker run --rm alpine cat /proc/version"
# Expected: Linux version 4.4.0 ...

# Verify new containers use runsc
ssh root@${RUN_NODE_IP} "docker run -d --name test-gvisor alpine sleep 60 && \
    docker inspect --format '{{.HostConfig.Runtime}}' test-gvisor && \
    docker rm -f test-gvisor"
# Expected: runsc

# Node.js test
ssh root@${RUN_NODE_IP} "docker run --rm node:20-alpine node -e 'console.log(process.version)'"
```

---

## Verification Checklist

Run the automated verification:

```bash
ssh root@${RUN_NODE_IP} "bash /root/verify-hardening.sh"
```

Expected output (all green):

```
=== Security Verification ===

1. gVisor:
  [OK] gVisor installed (runsc version release-20260126.0)
  [OK] gVisor is default runtime

2. Egress Rules:
  [OK] Cloud metadata endpoint blocked
  [OK] SMTP port 25 blocked
  [OK] Mining pool port 3333 blocked
  [OK] IRC port 6667 blocked

3. Miner Detection:
  [OK] Miner detection cron configured
  [OK] detect-miners.sh is executable

4. Docker Configuration:
  [OK] Docker live-restore enabled
  [OK] Docker daemon.json exists

5. System Containers:
  [OK] Reverse proxy (Traefik/coolify-proxy) is running
  [OK] Coolify services running

6. SSH Hardening:
  [OK] SSH password auth disabled
  [OK] SSH root login key-only

...
All critical security checks passed.
```

---

## Important Notes & Lessons Learned

### Runtime Behavior After Enabling gVisor Default

- **Existing containers** (started before the change): Continue using `runc`
- **New containers**: Automatically use `runsc` (gVisor)
- To check a container's runtime: `docker inspect --format '{{.HostConfig.Runtime}}' <container>`

### gVisor Kernel Emulation

Containers running under gVisor see an emulated kernel:

```
Linux version 4.4.0 #1 SMP Sun Jan 10 15:06:54 PST 2016
```

This is normal - gVisor intercepts syscalls and doesn't use the host kernel.

### Docker Restart Issues

If Docker fails to restart repeatedly, systemd may rate-limit it:

```bash
# Reset the failed state first
ssh root@${RUN_NODE_IP} "systemctl reset-failed docker.service && systemctl start docker.service"
```

### SSH Connection Drops

If SSH connections drop during setup, wait 5-10 seconds and retry. This can happen due to:

- Network latency
- Too many concurrent SSH sessions
- Server load during Docker restarts

### Coolify Proxy Naming

Coolify names its Traefik container `coolify-proxy`, not `traefik`. Both are the same thing.

---

## Troubleshooting

| Problem                                                      | Solution                                                                                                           |
| ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------ |
| Docker won't start after daemon.json change                  | Check logs: `journalctl -xeu docker.service`. Common issue: invalid JSON syntax                                    |
| App doesn't work under gVisor                                | Test with `--runtime=runc` to confirm. Some apps need raw sockets or io_uring                                      |
| Traefik/coolify-proxy down after restart                     | Containers with `live-restore: true` should survive. Check: `docker ps \| grep proxy`                              |
| SSH locked out after harden-ssh.sh                           | Use Hetzner Robot console to restore: `cp /etc/ssh/sshd_config.bak /etc/ssh/sshd_config && systemctl restart sshd` |
| "systemctl restart docker" rate-limited                      | Run `systemctl reset-failed docker.service` first                                                                  |
| gVisor test shows "command not found"                        | gVisor not installed. Run `install-gvisor.sh` again                                                                |
| Container shows runtime "runc" after enabling gVisor default | Expected for containers started before the change. New containers use runsc                                        |

---

## Rollback Procedures

### Revert gVisor to Non-Default

```bash
scp daemon-baseline.json root@${RUN_NODE_IP}:/etc/docker/daemon.json
ssh root@${RUN_NODE_IP} "systemctl restart docker"
```

### Remove Egress Rules

```bash
ssh root@${RUN_NODE_IP} "iptables -F DOCKER-USER && iptables -A DOCKER-USER -j RETURN && netfilter-persistent save"
```

### Revert SSH Hardening

```bash
ssh root@${RUN_NODE_IP} "cp /etc/ssh/sshd_config.bak /etc/ssh/sshd_config && systemctl restart sshd"
```

---

## Threat Model: What You're Actually Defending Against

| Threat                          | Likelihood | Impact               | Mitigation                      |
| ------------------------------- | ---------- | -------------------- | ------------------------------- |
| Crypto mining                   | High       | High (CPU/cost)      | CPU limits + process monitoring |
| Spam/phishing                   | High       | High (IP reputation) | Block SMTP ports                |
| DDoS launch point               | Medium     | High (IP blacklist)  | Rate limits + egress rules      |
| Container escape                | Low        | Critical             | gVisor                          |
| Credential theft (metadata)     | Medium     | Critical             | Block 169.254.169.254           |
| Fork bomb / resource exhaustion | Medium     | Medium               | pids-limit, memory limits       |
| Disk fill attack                | Medium     | Medium               | Ephemeral storage limits        |

---

## What Apps Should NOT Run (and How to Block)

### Explicitly Block at Deployment Time

| Blocked           | Detection                           | Action          |
| ----------------- | ----------------------------------- | --------------- |
| Crypto miners     | Dockerfile has xmrig, cgminer, etc. | Reject at build |
| Tor exit nodes    | Port 9001, 9030 in start command    | Reject          |
| VPN/proxy servers | OpenVPN, Wireguard, Shadowsocks     | Reject          |
| Torrenting        | qBittorrent, Transmission           | Reject          |
| Mail servers      | Postfix, Sendmail, SMTP apps        | Reject          |

**Practical implementation**: Scan Dockerfile and start commands for keywords before deploying.

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

### Runtime Monitoring (Detect & Log)

See `detect-miners.sh` - detects containers with sustained high CPU usage.

- Runs every 5 minutes via cron
- Logs to `/var/log/miner-detection.log`
- Auto-kill disabled by default (logging only)

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

| Tier   | Memory | CPU  | Ephemeral Disk | Use Case                 |
| ------ | ------ | ---- | -------------- | ------------------------ |
| Free   | 256m   | 0.25 | 512MB (tmpfs)  | Simple MCP servers       |
| Basic  | 512m   | 0.5  | 1GB (tmpfs)    | Most apps                |
| Pro    | 1g     | 1.0  | 2GB (tmpfs)    | Heavier workloads        |
| Custom | 2g+    | 2.0+ | 5GB+           | PDF processing, ML, etc. |

---

## Docker Daemon Configuration

### daemon.json (gVisor Default - Production)

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
    }
  }
}
```

**Note**: Do NOT add a `runc` entry to the runtimes - Docker already has runc built-in and adding it causes: `runtime name 'runc' is reserved`.

### daemon-baseline.json (gVisor Available, Not Default)

Use this for initial testing before making gVisor the default.

---

## System Containers (Use runc)

These containers run on runc (not gVisor) because they need special access:

| Container        | Why runc                                      |
| ---------------- | --------------------------------------------- |
| coolify-proxy    | Coolify's Traefik - needs full network access |
| coolify-sentinel | Coolify health monitoring                     |
| coolify-\*       | All Coolify internal services                 |
| cadvisor         | Needs /sys, /proc access for metrics          |
| alloy            | Grafana agent - needs host access             |

Since existing containers keep their runtime after changing the default, these system containers (started before gVisor was default) continue using runc automatically.

---

## What gVisor Protects Against

| Scenario                        | Without gVisor     | With gVisor                  |
| ------------------------------- | ------------------ | ---------------------------- |
| Kernel exploit (CVE-2024-21626) | Full host access   | Blocked - hits gVisor kernel |
| Container escape via runc bug   | Host compromise    | Blocked                      |
| /proc, /sys snooping            | Can read host info | Sandboxed                    |
| ptrace attacks                  | Possible           | Blocked                      |

**What gVisor does NOT protect against**:

- Network abuse (that's why you need egress rules)
- Resource exhaustion (that's why you need limits)
- Application-level vulns (your problem)

---

## Compatibility: What Breaks Under gVisor

| Works                           | Doesn't Work                     |
| ------------------------------- | -------------------------------- |
| Node.js, Python, Go, Rust, Java | Raw sockets (some network tools) |
| PostgreSQL, Redis, MongoDB      | io_uring (some high-perf libs)   |
| Next.js, Remix, FastAPI         | strace, gdb, perf                |
| WebSockets, HTTP/2              | Some kernel-specific syscalls    |

**Practical impact**: 95%+ of web apps work. If something breaks, user can report and you can investigate.

---

## Files in This Directory

| File                     | Purpose                                                  |
| ------------------------ | -------------------------------------------------------- |
| `README.md`              | This documentation                                       |
| `setup-run-node.sh`      | **One-command setup** for new run nodes (baseline)       |
| `setup-egress-rules.sh`  | Egress firewall rules (metadata, SMTP, mining, IRC, Tor) |
| `install-gvisor.sh`      | gVisor installation                                      |
| `detect-miners.sh`       | Runtime abuse detection (cron)                           |
| `verify-hardening.sh`    | Automated security verification                          |
| `harden-ssh.sh`          | SSH hardening (disable password auth)                    |
| `setup-host-firewall.sh` | UFW host firewall configuration (optional)               |
| `daemon.json`            | Docker daemon config with gVisor as default              |
| `daemon-baseline.json`   | Docker daemon config with gVisor available (not default) |

---

## Monitoring for Abuse

Check miner detection logs:

```bash
ssh root@${RUN_NODE_IP} "cat /var/log/miner-detection.log"
```

Add to Grafana Cloud alerts:

```promql
# High sustained CPU (potential miner)
avg_over_time(container_cpu_usage_seconds_total{container_name!~"traefik|cadvisor"}[5m]) > 0.9

# Unusual network egress
rate(container_network_transmit_bytes_total[5m]) > 10000000  # 10MB/s
```
