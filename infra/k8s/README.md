# k8s Base Manifests

These manifests are the declarative base for the Deploy MCP k3s cluster.

- `dp-system.yml` defines the system namespace.
- `deployer-server.yml` exposes webhook ingress (`api.ml.ink`) and `/healthz` for deployment control-plane traffic.
- `deployer-worker.yml` runs the `k8s-native` deployment worker queue.
- `registry.yml`, `registry-gc.yml`, `buildkit.yml` pin infra workloads to ops/build pools.
- `gvisor-runtimeclass.yml` constrains customer pods to run nodes.
- `wildcard-cert.yml` and `traefik-tlsstore.yml` set default wildcard TLS.
- `loki-ingress.yml` and `grafana-ingress.yml` expose observability endpoints through Traefik.

Secrets are provisioned by Ansible (`infra/ansible/roles/k8s_addons`) from inventory/vault vars.
Cloudflare LB is the source of truth for public ingress host/origin routing.

## Security Decisions

### Two security profiles: restricted vs compat

Customer pods run inside gVisor (`runtimeClassName: gvisor`), which is the real isolation boundary — it intercepts all syscalls in a userspace kernel. Capabilities and `allowPrivilegeEscalation` only affect gVisor's emulated kernel, not the host.

However, we still apply least-privilege **where we control the images** (defense in depth):

| Build pack | Profile | SecurityContext |
|---|---|---|
| `railpack`, `static` | **restricted** | `runAsNonRoot: true`, `drop: ALL` caps, `allowPrivilegeEscalation: false` |
| `dockerfile`, `dockercompose` | **compat** | `allowPrivilegeEscalation: false` only |

**Why two profiles:**
- `railpack`/`static` images are built by the platform — we can enforce non-root and dropped caps for free.
- `dockerfile` images are user-supplied. Dropping `ALL` capabilities removes `CAP_SETUID`/`CAP_SETGID`, which breaks standard images (nginx, postgres, redis) that start as root then drop privileges.
- `allowPrivilegeEscalation: false` (sets `no_new_privs`) is safe for both — it doesn't break images that start as root, only prevents gaining *additional* privileges via setuid binaries.

### Pod Security Standards: Baseline enforcement on customer namespaces

Customer namespaces carry `pod-security.kubernetes.io/enforce: baseline` labels. This is a K8s-native admission controller that acts as a safety net independent of the Go code in `k8sdeployments/`:
- **Allows**: root, Docker default capabilities, privilege escalation — needed for compatibility.
- **Blocks**: `privileged: true`, hostNetwork, hostPID, hostIPC, hostPath volumes, dangerous sysctls — things that would bypass gVisor or expose the host.

This ensures that even if the Go code has a bug, K8s itself will reject dangerous pod specs before they reach the kubelet.
