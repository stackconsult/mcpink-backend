# Infrastructure

MCPDeploy's execution plane. Builds and runs customer workloads on k3s clusters.

## Architecture

The system is split across two runtimes connected by Temporal workflows:

```
┌─────────────────────────────────────────────────────┐
│  Railway (product plane)                            │
│                                                     │
│  graphql (:8081)   API, GitHub OAuth, Firebase auth │
│  mcp (:8082)       MCP server, MCP OAuth            │
│  worker            Temporal worker (queue: default)  │
│  Postgres          System of record                 │
│  Temporal Cloud    Workflow orchestration            │
│                                                     │
│           │ Temporal workflows                       │
│           ▼                                         │
├─────────────────────────────────────────────────────┤
│  k3s cluster (execution plane)                      │
│                                                     │
│  deployer-server   Webhook receiver (GitHub)         │
│  deployer-worker   Build + deploy (queue: per-cluster)│
│  BuildKit          Container image builds           │
│  git-server        Internal Git (git.ml.ink)        │
│  Registry          Container image storage          │
│  Traefik           Customer ingress                 │
│  Prometheus/Loki   Observability                    │
│                                                     │
│  Customer pods     gVisor-sandboxed workloads       │
└─────────────────────────────────────────────────────┘
```

Railway owns API contracts, auth, metadata, and triggers deployment workflows. The k3s cluster executes builds, runs customer code, and serves customer traffic. The two planes communicate through Temporal Cloud — Railway's `worker` starts workflows, the cluster's `deployer-worker` runs the activities.

The `deployer-server` is NOT the product server — it only receives webhooks and health checks.

## Build → deploy pipeline

```
1. Customer pushes code
   ├── GitHub repo    → GitHub webhook    → Railway graphql → Temporal workflow
   └── Internal repo  → git-server post-receive → Temporal workflow directly

2. deployer-worker executes activities (queue: deployer-eu-central-1)
   clone       → Pull source from GitHub or internal git
   build       → BuildKit builds image (build-pool node)
   push        → Image → internal registry (registry.internal:5000)
   deploy      → Apply K8s resources: namespace, secret, deployment, service, ingress

3. Customer pod starts on run-pool
   Image pulled from internal registry
   Pod sandboxed by gVisor
   Traefik routes traffic via ingress rules

4. Optional: custom domain
   add_domain  → Returns TXT + CNAME instructions
   verify      → Checks both records, starts two-phase cert provisioning
   cert CR     → cert-manager issues via HTTP-01 (no Ingress yet)
   ingress     → TLS ingress created after cert is ready
```

Image naming: `registry.internal:5000/dp-{user_id}-{project}/{service}:{git_sha}`

## Network isolation

Each customer project is a K8s namespace (`dp-{user_id}-{project}`). Network policies enforce strict isolation.

**Within a namespace**: Services can discover and talk to each other via K8s DNS (e.g., `my-service` or `my-service.dp-user-project.svc.cluster.local`). This enables multi-service projects where a web app connects to its own database.

**Ingress policy**: Only Traefik (from `dp-system`) and pods within the same namespace can send traffic to customer pods. No cross-namespace traffic.

**Egress policy**: Customer pods can reach:

- DNS (port 53) — for service discovery and external lookups
- Public internet (any non-RFC1918 IP)

Customer pods CANNOT reach:

- Other customer namespaces
- Cluster-internal services (registry, git-server, K8s API)
- Cloud metadata endpoint (169.254.169.254)

## External dependencies

| Service        | What breaks if it's down                                                      | Required |
| -------------- | ----------------------------------------------------------------------------- | -------- |
| Temporal Cloud | No new deployments, deletes, or domain operations. Running pods unaffected.   | Yes      |
| Cloudflare     | `*.ml.ink` traffic stops. Custom domains unaffected (they bypass CF).         | Yes      |
| GitHub         | No OAuth login, no webhook deploys from GitHub repos. Internal git repos unaffected. | Yes      |
| Let's Encrypt  | New custom domain certs can't be issued. Existing certs work until expiry.    | Yes      |
| Railway        | API, MCP server, and product DB offline. Running customer pods unaffected.    | Yes      |
| Firebase       | Token validation fails for Firebase-auth users.                               | Optional |

**Key insight**: Running customer pods survive any single dependency failure. Only new operations (deploys, deletes, logins) are affected.

## Stateful vs stateless

Understanding what's stateful is critical for disaster recovery.

| Component                   | Stateful?  | What you lose                                   | Recovery                                                        |
| --------------------------- | ---------- | ----------------------------------------------- | --------------------------------------------------------------- |
| Customer pods (run-pool)    | No         | Apps restart, no data loss                      | Redeploy from registry images                                   |
| BuildKit cache (build-pool) | Cache only | Slower rebuilds                                 | Rebuilds from scratch                                           |
| K3s etcd (ctrl)             | Yes        | All K8s state (deployments, secrets, ingresses) | Restore from etcd snapshot or re-run `site.yml` + redeploy apps |
| Registry (ops)              | Yes        | All built container images                      | Rebuild from source (slow but possible)                         |
| git-server (ops)            | Yes        | Internal Git repos, bare repos on disk          | Repos are source of truth for `host=ml.ink` — back up `/mnt/git-repos` |
| Observability (ops)         | Yes        | Metrics (30d) + logs (30d) + dashboards         | Redeploy with empty state                                       |

**ops node is the stateful heart.** It has RAID1 for redundancy but no off-node backups yet.

## Directory structure

```
infra/
├── ansible/              Shared automation (roles, playbooks)
│   ├── roles/            Reusable roles — provider-agnostic where possible
│   ├── playbooks/        Cluster lifecycle playbooks
│   └── README.md         Operations runbook
│
└── <region>/             Per-cluster state (one directory = one cluster)
    ├── inventory/        Ansible inventory (hosts, group_vars, vault)
    ├── k8s/              Kubernetes manifests and Helm values
    ├── known_hosts       SSH host keys
    └── README.md         Region-specific: machines, network, topology, decisions
```

## Regions

| Region         | Provider                    | Status     |
| -------------- | --------------------------- | ---------- |
| `eu-central-1` | Hetzner (Cloud + Dedicated) | Production |

## Global requirements

These apply to all regions regardless of provider.

### Cloudflare Load Balancer

All `*.ml.ink` traffic routes through a Cloudflare LB. Each region's run-pool nodes are origins.

- Managed in Cloudflare dashboard (health checks, failover, TLS termination)
- Origin pool updated manually when adding/removing run nodes

### DNS

| Record              | Type              | Target              | Purpose                                 |
| ------------------- | ----------------- | ------------------- | --------------------------------------- |
| `*.ml.ink`          | Proxied via CF LB | Run-pool origins    | Customer app subdomains                 |
| `grafana.ml.ink`    | Proxied via CF LB | Run-pool origins    | Monitoring                              |
| `loki.ml.ink`       | Proxied via CF LB | Run-pool origins    | Log aggregation                         |
| `prometheus.ml.ink` | Proxied via CF LB | Run-pool origins    | Metrics                                 |
| `cname.ml.ink`      | A (DNS-only)      | Region LB public IP | Custom domain CNAME target (legacy)     |
| `*.cname.ml.ink`    | A (DNS-only)      | Region LB public IP | Per-service custom domain CNAME targets |

### Custom domains

Each service gets a per-service CNAME target: `{service-name}.cname.ml.ink`. Users must add both a TXT record (`_dp-verify.{domain}` for ownership proof) and a CNAME record pointing to the per-service target.

Each region must provide a TCP passthrough LB (ports 80, 443) that forwards to run-pool nodes. The `*.cname.ml.ink` wildcard A record points to that LB (DNS-only, not proxied). For multi-region, explicit per-service A records override the wildcard to route services to specific clusters.

cert-manager handles TLS via HTTP-01 challenges using a two-phase approach: Certificate CR created first (no Ingress), then Ingress with TLS added after cert is ready. This avoids Traefik v3's automatic HTTP→HTTPS redirect that would block the HTTP-01 solver.

### K8s conventions

- Customer namespaces: `dp-{user_id}-{project}`
- Customer pods MUST use `runtimeClassName: gvisor` — provides sandbox AND pins to run-pool nodes via nodeSelector
- Templates in `<region>/k8s/templates/` are the design spec — Go code in `k8sdeployments/` must match them

### cert-manager

- `letsencrypt-prod` ClusterIssuer — HTTP-01 for custom domain TLS
- Cloudflare DNS-01 solver — wildcard `*.ml.ink` certificates

## Platform decisions

These are settled product-level decisions that apply to all regions.

| Decision                                      | Rationale                                                                                                                                                                                                                              |
| --------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| gVisor for all customer pods                  | Runs untrusted user code — kernel-level sandbox prevents container escapes                                                                                                                                                             |
| No capability dropping, root allowed in pods  | gVisor is the security boundary (all syscalls hit userspace kernel, not host). Dropping caps or forcing non-root breaks postgres, nginx, redis, and similar software that needs CAP_SETUID/CAP_SETGID. Same model as Railway. Settled. |
| gVisor RuntimeClass carries nodeSelector      | Guarantees customer pods only land on run-pool nodes, not the control plane                                                                                                                                                            |
| TCP passthrough LB for custom domains         | cert-manager needs raw HTTP-01 on port 80 — TLS-terminating LB would break it                                                                                                                                                          |
| Firewall source-restricts 80/443 on run nodes | Prevents direct-to-IP attacks; only region LB and Cloudflare can reach Traefik                                                                                                                                                         |
| SMTP egress blocked on all nodes              | Prevents spam abuse from customer workloads                                                                                                                                                                                            |
| Cloudflare for `*.ml.ink`                     | DDoS protection, CDN, health-checks for platform-managed subdomains                                                                                                                                                                    |

## gVisor memory overhead

Measured on run-1 (release-20260209.1, systrap platform, bare-metal EPYC) with `systemd-cgroup=true`.

### Per-pod overhead (cgroup-measured, Feb 2026)

| Workload            | Without gVisor | With gVisor (cgroup) | Overhead                          |
| ------------------- | -------------- | -------------------- | --------------------------------- |
| Static HTML (nginx) | ~44Mi          | ~36Mi                | **-8Mi** (page cache constrained) |
| Next.js app         | ~90Mi          | ~136Mi               | **~46Mi**                         |

Overhead aligns with gVisor's published density benchmarks: ~45-60Mi for small web services.

### Why `kubectl top` and CRI stats overcount

`kubectl top` uses CRI stats (`job=kubelet-resource`), which for gVisor includes **directfs Mapped page cache**. This is host page cache from mmapped container files (node_modules, etc.) that the Sentry accesses via directfs. These pages are **not charged to the container's cgroup** — they live in the host page cache.

| Metric source              | malaysia-app | What it measures                            |
| -------------------------- | ------------ | ------------------------------------------- |
| `cgroup memory.current`    | 136Mi        | Actual kernel-tracked memory in pod cgroup  |
| `kubectl top` (CRI)        | ~300Mi       | Includes ~214Mi reclaimable host page cache |
| `cAdvisor` (`job=kubelet`) | 136Mi        | Reads cgroup directly (accurate)            |

The GraphQL API uses cAdvisor metrics (`job=kubelet, container=""`) for accurate reporting.

### Page cache reclaim (automatic)

The 214Mi of Mapped page cache that runsc reports is **automatically reclaimable** by the Linux kernel:

1. **Cgroup pressure**: When a pod approaches `memory.max`, the kernel evicts least-recently-used file pages from the container's charged pages
2. **Global pressure**: When the node needs memory, the kernel reclaims from the host page cache (where directfs pages live)
3. **No action needed**: This is standard Linux page cache behavior — pages are cached for performance and evicted on demand

With `systemd-cgroup=true`, the sandbox joins `kubepods.slice` with the correct `memory.max`. This constrains the charged memory (anon + shmem + kernel) and the kernel manages eviction automatically.

### RuntimeClass overhead: 64Mi

The RuntimeClass specifies `overhead.podFixed.memory=64Mi`. This tells the scheduler:

```
pod memory.max = container_limit + 64Mi
```

64Mi covers the Sentry's baseline cost (~35Mi measured) with ~29Mi buffer. This overhead is **not wasted** — it's the memory budget for the gVisor Sentry process (Go runtime, kernel data structures, page tables).

### Configuration (`runsc.toml`)

```toml
[runsc_config]
systemd-cgroup = "true"   # Fix: sandbox joins kubepods.slice, not system.slice
overlay2 = "root:self"    # Host-backed overlay (not memory-backed)
# platform defaults to systrap — best for density
```

**Why systrap, not KVM**: KVM adds ~24Mi kernel overhead per pod (pagetables, sec_pagetables, vmalloc for address space mapping). Systrap uses ~4Mi. KVM gives 10x lower CPU usage but costs 6x more kernel memory. For density-sensitive workloads, systrap wins. KVM is only worth it for CPU-bound workloads where syscall latency matters.

### Key numbers (all values must be strings in runsc.toml)

| Setting               | Value                | Effect                                       |
| --------------------- | -------------------- | -------------------------------------------- |
| `systemd-cgroup`      | `"true"`             | Sandbox cgroup in kubepods (enforced limits) |
| `overlay2`            | `"root:self"`        | Host-backed tmpfs for overlay writes         |
| Platform              | systrap (default)    | ~4Mi kernel overhead vs ~28Mi for KVM        |
| RuntimeClass overhead | 64Mi memory, 50m CPU | Scheduler reserves for Sentry                |

## Adding a new region

Rough checklist — a new provider will almost certainly require additional steps not listed here.

1. Create `infra/<region>/` with inventory, k8s manifests, known_hosts
2. Provider-specific roles go in `ansible/roles/` with clear naming (e.g., `hetzner_lb`)
3. Provider-agnostic roles (firewall, gvisor, k3s\_\*) should work unchanged
4. Set up private networking between nodes (provider-specific)
5. Provision a TCP passthrough LB for custom domain traffic (ports 80, 443 → run-pool)
6. Add `*.cname.ml.ink` A record pointing to the new LB (for multi-region: explicit per-service records override wildcard)
7. Add run-pool node IPs to Cloudflare LB origin pool
8. Configure firewall to restrict 80/443 to LB + Cloudflare CIDRs
9. Document provider specifics and decisions in `<region>/README.md`
