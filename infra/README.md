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
│  deployer-server   Webhook receiver (GitHub/Gitea)  │
│  deployer-worker   Build + deploy (queue: k8s-native)│
│  BuildKit          Container image builds           │
│  Gitea             Internal Git mirror              │
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
   ├── GitHub repo → GitHub webhook → Railway graphql → Temporal workflow
   └── Gitea repo  → Gitea webhook  → deployer-server → Temporal workflow

2. deployer-worker executes activities (queue: k8s-native)
   clone       → Pull source from GitHub/Gitea
   build       → BuildKit builds image (build-pool node)
   push        → Image → internal registry (registry.internal:5000)
   deploy      → Apply K8s resources: namespace, secret, deployment, service, ingress

3. Customer pod starts on run-pool
   Image pulled from internal registry
   Pod sandboxed by gVisor
   Traefik routes traffic via ingress rules

4. Optional: custom domain
   add_domain  → Create custom domain ingress + cert-manager TLS
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
- Cluster-internal services (registry, Gitea, K8s API)
- Cloud metadata endpoint (169.254.169.254)

## External dependencies

| Service | What breaks if it's down | Required |
|---------|--------------------------|----------|
| Temporal Cloud | No new deployments, deletes, or domain operations. Running pods unaffected. | Yes |
| Cloudflare | `*.ml.ink` traffic stops. Custom domains unaffected (they bypass CF). | Yes |
| GitHub | No OAuth login, no webhook deploys from GitHub repos. Gitea repos unaffected. | Yes |
| Let's Encrypt | New custom domain certs can't be issued. Existing certs work until expiry. | Yes |
| Railway | API, MCP server, and product DB offline. Running customer pods unaffected. | Yes |
| Firebase | Token validation fails for Firebase-auth users. | Optional |

**Key insight**: Running customer pods survive any single dependency failure. Only new operations (deploys, deletes, logins) are affected.

## Stateful vs stateless

Understanding what's stateful is critical for disaster recovery.

| Component | Stateful? | What you lose | Recovery |
|-----------|-----------|---------------|----------|
| Customer pods (run-pool) | No | Apps restart, no data loss | Redeploy from registry images |
| BuildKit cache (build-pool) | Cache only | Slower rebuilds | Rebuilds from scratch |
| K3s etcd (ctrl) | Yes | All K8s state (deployments, secrets, ingresses) | Restore from etcd snapshot or re-run `site.yml` + redeploy apps |
| Registry (ops) | Yes | All built container images | Rebuild from source (slow but possible) |
| Gitea (ops) | Yes | Internal Git mirrors, webhook config | Re-mirror from GitHub |
| Observability (ops) | Yes | Metrics (30d) + logs (30d) + dashboards | Redeploy with empty state |

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

| Region | Provider | Status |
|--------|----------|--------|
| `eu-central-1` | Hetzner (Cloud + Dedicated) | Production |

## Global requirements

These apply to all regions regardless of provider.

### Cloudflare Load Balancer

All `*.ml.ink` traffic routes through a Cloudflare LB. Each region's run-pool nodes are origins.

- Managed in Cloudflare dashboard (health checks, failover, TLS termination)
- Origin pool updated manually when adding/removing run nodes

### DNS

| Record | Type | Target | Purpose |
|--------|------|--------|---------|
| `*.ml.ink` | Proxied via CF LB | Run-pool origins | Customer app subdomains |
| `grafana.ml.ink` | Proxied via CF LB | Run-pool origins | Monitoring |
| `loki.ml.ink` | Proxied via CF LB | Run-pool origins | Log aggregation |
| `prometheus.ml.ink` | Proxied via CF LB | Run-pool origins | Metrics |
| `cname.ml.ink` | A (DNS-only) | Region LB public IP | Custom domain CNAME target |

### Custom domains

Customers point `CNAME → cname.ml.ink`. Each region must provide a TCP passthrough LB (ports 80, 443) that forwards to run-pool nodes. The `cname.ml.ink` A record points to that LB. cert-manager handles TLS via HTTP-01 challenges.

### K8s conventions

- Customer namespaces: `dp-{user_id}-{project}`
- Customer pods MUST use `runtimeClassName: gvisor` — provides sandbox AND pins to run-pool nodes via nodeSelector
- Templates in `<region>/k8s/templates/` are the design spec — Go code in `k8sdeployments/` must match them

### cert-manager

- `letsencrypt-prod` ClusterIssuer — HTTP-01 for custom domain TLS
- Cloudflare DNS-01 solver — wildcard `*.ml.ink` certificates

## Platform decisions

These are settled product-level decisions that apply to all regions.

| Decision | Rationale |
|----------|-----------|
| gVisor for all customer pods | Runs untrusted user code — kernel-level sandbox prevents container escapes |
| No capability dropping, root allowed in pods | gVisor is the security boundary (all syscalls hit userspace kernel, not host). Dropping caps or forcing non-root breaks postgres, nginx, redis, and similar software that needs CAP_SETUID/CAP_SETGID. Same model as Railway. Settled. |
| gVisor RuntimeClass carries nodeSelector | Guarantees customer pods only land on run-pool nodes, not the control plane |
| TCP passthrough LB for custom domains | cert-manager needs raw HTTP-01 on port 80 — TLS-terminating LB would break it |
| Firewall source-restricts 80/443 on run nodes | Prevents direct-to-IP attacks; only region LB and Cloudflare can reach Traefik |
| SMTP egress blocked on all nodes | Prevents spam abuse from customer workloads |
| Cloudflare for `*.ml.ink` | DDoS protection, CDN, health-checks for platform-managed subdomains |

## Adding a new region

Rough checklist — a new provider will almost certainly require additional steps not listed here.

1. Create `infra/<region>/` with inventory, k8s manifests, known_hosts
2. Provider-specific roles go in `ansible/roles/` with clear naming (e.g., `hetzner_lb`)
3. Provider-agnostic roles (firewall, gvisor, k3s_*) should work unchanged
4. Set up private networking between nodes (provider-specific)
5. Provision a TCP passthrough LB for custom domain traffic (ports 80, 443 → run-pool)
6. Update `cname.ml.ink` A record to point to the new LB (or set up multi-region DNS)
7. Add run-pool node IPs to Cloudflare LB origin pool
8. Configure firewall to restrict 80/443 to LB + Cloudflare CIDRs
9. Document provider specifics and decisions in `<region>/README.md`
