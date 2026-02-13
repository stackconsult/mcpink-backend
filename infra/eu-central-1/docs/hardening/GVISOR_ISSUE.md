# gVisor on run nodes - RESOLVED

## Status: ✅ WORKING

gVisor is now protecting user containers with kernel-level isolation.

---

## Solution Summary

### What Works

| Component | Status | Details |
|-----------|--------|---------|
| gVisor runtime | ✅ | `runsc` with hostinet mode |
| DNS resolution | ✅ | External (github.com) and internal (coolify-proxy) |
| Container networking | ✅ | Private IPs (10.x.x.x), Traefik routing works |
| Coolify integration | ✅ | Via `--runtime=runsc` in Custom Docker Run Options |

### Configuration

**1. Docker daemon.json on run node** (`/etc/docker/daemon.json`):
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
  "runtimes": {
    "runsc": {
      "path": "/usr/local/bin/runsc",
      "runtimeArgs": ["--network=host"]
    }
  }
}
```

**Key insight:** `runtimeArgs: ["--network=host"]` enables **hostinet mode**, which:
- Uses kernel networking instead of gVisor's netstack
- Allows Docker's DNS iptables rules to work
- Container still has private Docker IP (NOT host network namespace)

**2. Coolify Custom Docker Run Options:**
```
--runtime=runsc
```

This required a patch to Coolify (PR: https://github.com/coollabsio/coolify/pull/8113) because `--runtime` was not in their supported options list.

---

## What We Tried (and Why It Failed)

### Attempt 1: gVisor as Default Runtime

**Configuration:**
```json
{
  "default-runtime": "runsc",
  "runtimes": { "runsc": { "path": "/usr/local/bin/runsc" } }
}
```

**Result:** ❌ Failed

**Why:** Coolify's helper container mounts `/var/run/docker.sock` and runs `docker compose` inside. gVisor blocks Unix socket access to the host (security feature), breaking deployments.

**Error:**
```
Cannot connect to the Docker daemon at unix:///var/run/docker.sock
```

### Attempt 2: gVisor Default + Hostinet

**Configuration:**
```json
{
  "default-runtime": "runsc",
  "runtimes": { "runsc": { "path": "/usr/local/bin/runsc", "runtimeArgs": ["--network=host"] } }
}
```

**Result:** ❌ Failed (same error)

**Why:** Hostinet only affects networking, not Unix socket access. gVisor still blocks Docker socket.

### Attempt 3: Coolify Custom Docker Run Options

**Configuration:** Set `--runtime=runsc` in Coolify UI

**Result:** ❌ Failed initially

**Why:** Coolify filters custom docker run options. `--runtime` was not in the allowed list:
- Supported: `--ip`, `--ip6`, `--shm-size`, `--cap-add`, `--cap-drop`, `--security-opt`, `--sysctl`, `--device`, `--ulimit`, `--init`, `--privileged`, `--gpus`, `--entrypoint`
- NOT supported: `--runtime`

### Attempt 4: Patch Coolify + Hostinet (SUCCESS)

**Steps:**
1. Forked Coolify and added `--runtime` support to `bootstrap/helpers/docker.php`
2. Copied patched file into running Coolify container on Factory
3. Set `--runtime=runsc` in app's Custom Docker Run Options
4. Redeployed

**Result:** ✅ Works!

---

## Verification

```bash
# Check container runtime
docker inspect <container> --format '{{.HostConfig.Runtime}}'
# Output: runsc

# Verify gVisor kernel (inside container)
docker exec <container> cat /proc/version
# Output: Linux version 4.4.0 #1 SMP Sun Jan 10 15:06:54 PST 2016

# Test DNS
docker exec <container> nslookup github.com
# Output: resolves correctly

# Check container has private IP
docker exec <container> hostname -i
# Output: 10.x.x.x (NOT host IP)
```

---

## Security Analysis

### What gVisor Protects

| Attack Vector | Protection |
|---------------|------------|
| Kernel exploits (CVEs) | ✅ Syscalls intercepted by gVisor's Sentry |
| Container escapes | ✅ Isolated filesystem, process, memory namespaces |
| `/proc` `/sys` attacks | ✅ gVisor provides synthetic procfs/sysfs |
| Raw socket sniffing | ✅ Blocked by default (no `--net-raw`) |

### What Hostinet Exposes

| Exposure | Mitigation |
|----------|------------|
| Network syscalls go through host kernel | Kernel is hardened, capabilities dropped |
| Theoretical network stack exploits | Very rare; standard kernel security applies |

### Trade-off Assessment

**Hostinet is acceptable because:**
1. Network exploits are far rarer than filesystem/process exploits
2. Container still has `--cap-drop=ALL` (no raw sockets, no packet sniffing)
3. Egress firewall rules still active
4. This is the gVisor team's recommended approach for Docker integration

---

## Files Changed

| File | Change |
|------|--------|
| `infra/hetzner/hardening/daemon.json` | Added hostinet mode to runsc runtime |
| Coolify (patched) | Added `--runtime` to supported options |

---

## Deployment Checklist

For new run nodes:

- [ ] Install gVisor: https://gvisor.dev/docs/user_guide/install/
- [ ] Configure `/etc/docker/daemon.json` with hostinet mode
- [ ] Restart Docker: `systemctl restart docker`
- [ ] Verify: `docker run --runtime=runsc alpine cat /proc/version` shows `4.4.0`
- [ ] Patch Coolify (until PR is merged): Copy `docker.php` to container
- [ ] Set `--runtime=runsc` in app Custom Docker Run Options

---

## References

- [gVisor Networking Docs](https://gvisor.dev/docs/user_guide/networking/) - Explains hostinet vs netstack
- [Coolify PR #8113](https://github.com/coollabsio/coolify/pull/8113) - Adds `--runtime` support
- [gVisor DNS Issue #7469](https://github.com/google/gvisor/issues/7469) - Why netstack breaks Docker DNS
