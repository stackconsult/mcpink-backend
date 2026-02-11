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

### Container SecurityContext: No capability dropping (gVisor boundary)

Customer pods run under gVisor (`runtimeClassName: gvisor`), which intercepts all syscalls in a userspace kernel. Capabilities, seccomp, AppArmor, and `allowPrivilegeEscalation` only affect gVisor's emulated kernel — they provide zero additional host-escape protection. GKE Sandbox (Google's production gVisor product) confirms seccomp/AppArmor are incompatible with gVisor.

We intentionally do **not** drop capabilities or set `allowPrivilegeEscalation: false` because:
- It breaks standard Docker images (nginx, postgres, redis) that need root or SETUID/SETGID inside the container.
- The restrictions only affect processes inside gVisor's sandbox, not the host kernel.
- gVisor itself is the isolation boundary.

### Pod Security Standards: Baseline enforcement on customer namespaces

Customer namespaces carry `pod-security.kubernetes.io/enforce: baseline` labels. This is a K8s-native admission controller that acts as a safety net independent of the Go code in `k8sdeployments/`:
- **Allows**: root, Docker default capabilities, privilege escalation — needed for compatibility.
- **Blocks**: `privileged: true`, hostNetwork, hostPID, hostIPC, hostPath volumes, dangerous sysctls — things that would bypass gVisor or expose the host.

This ensures that even if the Go code has a bug, K8s itself will reject dangerous pod specs before they reach the kubelet.
