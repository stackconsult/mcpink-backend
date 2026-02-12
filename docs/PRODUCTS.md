# Competitive Landscape & Platform Analysis

> Research doc for Ink MCP (ml.ink) — understanding what exists, what's hard, what's easy, and where the opportunity is.

---

## Platform Comparison Table

| Feature | Railway | Fly.io | Render | Vercel | Heroku | DigitalOcean App Platform | Coolify | **Ink MCP (ml.ink)** |
|---|---|---|---|---|---|---|---|---|
| **Deploy from repo** | Yes | Yes | Yes | Yes | Yes | Yes | Yes | **Yes** |
| **Auto-deploy on push** | GitHub | GitHub | GitHub/GitLab | GitHub/GitLab | GitHub | GitHub/GitLab | GitHub/GitLab/Gitea | **GitHub + Gitea** |
| **Dockerfile support** | Yes | Yes (required) | Yes | No | Yes | Yes | Yes | **Yes** |
| **Auto-detect buildpack** | Railpack | Dockerfile only | Nixpacks | Next.js-centric | Heroku buildpacks | Buildpacks | Nixpacks/Railpack | **Railpack** |
| **Persistent volumes** | Yes | Yes | Yes (disk) | No | No (ephemeral only) | No | Yes | **Not yet** |
| **Managed databases** | Postgres, MySQL, Redis, MongoDB | Postgres, Redis | Postgres, Redis | Postgres (via Neon) | Postgres, Redis | Postgres, MySQL, Redis, MongoDB | Postgres, MySQL, Redis, MongoDB | **SQLite (Turso)** |
| **Private networking** | Wireguard mesh (IPv6) | Wireguard mesh | Private services | No | Private Spaces ($$$) | Yes | Docker networks | **K8s Services (DNS)** |
| **Custom domains** | Yes + wildcard | Yes | Yes | Yes | Yes | Yes | Yes | **Not yet (wildcard only)** |
| **SSL/TLS** | Auto (Let's Encrypt) | Auto | Auto | Auto | Auto | Auto | Auto (Let's Encrypt) | **Auto (cert-manager wildcard)** |
| **Health checks** | Deploy-time only | Continuous | Continuous | N/A (serverless) | No | Yes | No | **Not yet (k8s supports continuous)** |
| **Horizontal scaling** | Replicas (up to 42) | Machines per region | Manual instances | Auto (serverless) | Manual dynos | Manual instances | No | **Not yet (trivial to add)** |
| **Multi-region** | 4 regions | 18 regions | No | 70+ edge PoPs | No | 8 regions | No | **1 region (FSN1)** |
| **Cron jobs** | Yes | Yes (via Machines) | Yes | Cron (paid) | Scheduler add-on | No | Yes | **Not yet** |
| **Environments** | Prod + staging + PR envs | Separate apps | Yes + PR previews | Preview deployments | Pipelines | No native | No | **Not yet** |
| **Config-as-code** | `railway.toml` | `fly.toml` | `render.yaml` | `vercel.json` | `app.json` + `Procfile` | `app.yaml` | No | **Not yet** |
| **Rollbacks** | Instant | Yes | Yes | Instant | Yes | No | Yes | **Not yet** |
| **Logs** | Build + runtime + HTTP | Runtime | Build + runtime | Function logs | Runtime (Logplex) | Build + runtime | Yes | **Build + deploy (Loki for runtime)** |
| **Metrics** | CPU/RAM/Network | Prometheus + Grafana | CPU/RAM/Bandwidth | Function metrics | Basic | CPU/RAM | No | **Prometheus + Grafana (not exposed via MCP yet)** |
| **CLI** | `railway` (30+ commands) | `flyctl` (extensive) | `render` CLI | `vercel` CLI | `heroku` CLI | `doctl` | No | **MCP tools (12 tools)** |
| **GraphQL/REST API** | GraphQL API | REST Machines API | REST API | REST API | REST API (Platform) | REST API | No | **GraphQL + MCP** |
| **Webhooks (events)** | Yes | No | No | Yes | Yes (deploy hooks) | No | Yes | **Incoming (GitHub/Gitea push)** |
| **TCP proxy** | Yes | Native (any port) | Yes (paid) | No | No | No | Yes | **Not yet** |
| **Static IPs** | Outbound only | No | Yes (paid) | No | Private Spaces | No | N/A (your own IPs) | **Single node IP** |
| **Serverless / scale-to-zero** | Yes (sleep after 10min) | Yes (autostart/autostop) | No | Yes (native model) | Eco dynos (sleep) | No | No | **Not yet** |
| **SSH into container** | Yes | Yes | Yes | No | Yes (one-off dynos) | Yes (console) | Yes | **Not yet** |
| **Pre-deploy commands** | Yes (migrations) | No (use release command) | Yes | No | Release phase | No | No | **Not yet** |
| **Docker Compose** | No | No | No | No | No | No | Yes | **Not yet** |
| **Template marketplace** | Yes (50% kickback) | No | No | Templates | Buttons | 1-click apps | One-click services | **Not yet** |

---

## Underlying Technology

| Platform | Container Runtime | Orchestration | Infrastructure | Networking | Build System |
|---|---|---|---|---|---|
| **Railway** | Custom containers (OCI) | **Custom orchestrator** (Temporal-based, no k8s) | Own bare metal in 4 DCs (US-W, US-E, EU-W, SEA) | eBPF + Wireguard IPv6 mesh, anycast edge | Railpack (custom, replaced Nixpacks) |
| **Fly.io** | **Firecracker microVMs** | Custom scheduler | Own bare metal in 18+ regions | Rust `fly-proxy` + Wireguard tunnels between DCs | User provides Dockerfile (no buildpack) |
| **Render** | Docker containers | **Kubernetes** (abstracted) | AWS + GCP | K8s networking + internal DNS | Nixpacks (or Dockerfile) |
| **Vercel** | **AWS Lambda functions** | Serverless (no containers) | AWS (Lambda + S3 + CloudFront) | Edge network (70+ PoPs), custom TCP tunnel to Lambda | Next.js compiler / Turbopack |
| **Heroku** | Linux containers (dynos) | **Custom** (dyno manager on EC2) | AWS EC2 | Custom routing mesh (bamboo → router → dyno) | Heroku buildpacks (classic) |
| **DigitalOcean** | Docker containers | **Kubernetes** (managed DOKS) | DigitalOcean's own DCs (8 regions) | K8s networking | Buildpacks (or Dockerfile) |
| **Coolify** | Docker containers | **Docker / Docker Swarm** | Your own servers (SSH) | Docker networks + Traefik | Nixpacks / Railpack / Dockerfile |
| **Ink MCP** | Docker containers (gVisor) | **k3s (Kubernetes)** | Hetzner bare metal + cloud (1 region) | K8s Services + Traefik + Cloudflare LB | BuildKit + Railpack |

### Key Takeaways

**Who uses Kubernetes:**
- Render — k8s under the hood, abstracted away
- DigitalOcean — managed k8s (DOKS)
- Ink MCP (us) — k3s

**Who built custom:**
- Railway — burned a year trying k8s, abandoned it, built custom orchestrator on Temporal
- Fly.io — Firecracker microVMs with custom Rust scheduler, most technically ambitious
- Heroku — legacy dyno manager on EC2, now transitioning to ARM-based "Fir" generation
- Vercel — not even containers, pure serverless on AWS Lambda

**Who wraps existing tools:**
- Coolify — Docker + Traefik + Nixpacks, open-source, closest to our architecture pattern

---

## What Each Platform Invested Heavily In (That Was Hard)

### Railway
1. **Custom orchestrator replacing k8s** — years of work after k8s attempt nearly killed the company
2. **Own bare metal DCs** — 4 global locations, custom provisioning (MetalCP + Temporal), BGP/CLOS networking
3. **Anycast edge network** — BGP peering from their own ASN
4. **Railpack build system** — replaced Nixpacks with their own builder
5. **Developer experience layer** — collaborative canvas UI, template marketplace with revenue sharing

### Fly.io
1. **Firecracker integration** — running AWS's microVM tech on their own bare metal, custom Rust init
2. **18-region bare metal footprint** — most global coverage of any indie PaaS
3. **Rust proxy layer** (`fly-proxy`) — handles all routing, TLS termination, inter-region backhaul
4. **Machines API** — low-level VM control (start/stop/resize individual machines)

### Vercel
1. **Edge network** — 70+ PoPs, custom protocol between edge and Lambda
2. **Next.js integration** — they own the framework AND the hosting, tight coupling
3. **Fluid Compute** — custom request multiplexing layer on top of Lambda to reduce cold starts
4. **Framework-aware builds** — Turbopack, ISR, streaming SSR

### Render
1. **Kubernetes abstraction** — making k8s invisible to users while using it underneath
2. **Blueprint system** — `render.yaml` as infrastructure-as-code
3. **Managed databases** — Postgres with automated backups, Redis, point-in-time recovery

### Heroku
1. **Buildpacks** (invented the concept) — now a CNCF standard
2. **Add-on marketplace** — hundreds of third-party integrations
3. **Salesforce integration** — Heroku Connect for bi-directional Salesforce sync

---

## What's Easy vs Hard for Ink MCP

### Easy (K8s gives it to us, just wire it up)

| Feature | K8s Primitive | Work |
|---|---|---|
| Health checks (continuous) | `readinessProbe` + `livenessProbe` | Add to pod spec in `k8s_resources.go` |
| Wire resource limits | `resources.limits` / `resources.requests` | Already stored in DB, just connect to pod spec |
| Horizontal scaling (replicas) | `spec.replicas` | Expose in MCP tool + GraphQL |
| Cron jobs | `CronJob` resource | New resource type in `k8s_resources.go` |
| Persistent volumes | `PersistentVolumeClaim` + k3s `local-path` provisioner | Add PVC creation, expose mount path in MCP |
| Pre-deploy commands | Init containers | Add initContainer to pod spec |
| Deployment overlap / draining | `terminationGracePeriodSeconds` + `minReadySeconds` | Pod spec fields |
| Rollbacks | `kubectl rollout undo` or redeploy previous image tag | Track image history, add MCP tool |
| Restart policy tuning | `restartPolicy` + `backoffLimit` | Pod spec fields |
| Private networking (DNS) | K8s Service DNS (`svc.cluster.local`) | Already works, just expose the DNS name |

### Medium (A few days of work each)

| Feature | What's Needed | Work |
|---|---|---|
| Custom domains | Traefik IngressRoute per domain + cert-manager HTTP-01 | DNS verification + per-domain cert issuance |
| Managed Postgres | Deploy Postgres container + PVC, return connection string | Template Deployment + Service + PVC + Secret |
| Managed Redis | Deploy Redis container + PVC | Same pattern as Postgres |
| Runtime logs via MCP | Query Loki API by pod labels | Already have Loki, need API integration |
| Runtime metrics via MCP | Query Prometheus by pod labels | Already have Prometheus, need API integration |
| TCP proxy (non-HTTP) | Traefik TCP IngressRoute or NodePort | Port allocation management is the tricky part |
| SSH into container | `kubectl exec` via API | Auth + session management |
| Config-as-code | Read `deploy.toml` from repo during build | Parse config, merge with MCP tool params |

### Hard (Significant infrastructure investment)

| Feature | Why It's Hard | Feasibility |
|---|---|---|
| Multi-region | Need servers in other locations, cross-region networking, geo-routing | Could use Cloudflare Workers for edge, but actual multi-region compute needs more Hetzner nodes |
| Scale-to-zero / serverless | K8s can scale to 0 replicas but needs traffic-triggered wake-up. KEDA or Knative Serving could do this, but adds complexity to the cluster | Medium-hard — Knative or custom controller |
| Anycast / edge network | Need own ASN, BGP peering | Not feasible — use Cloudflare instead |
| PR preview environments | Auto-create namespace on PR open, deploy, teardown on merge | Medium-hard — GitHub App PR event handling + lifecycle management |
| Template marketplace | Curated templates, one-click deploy, revenue sharing | Product work more than infrastructure |
| Environments (staging/prod) | Namespace-per-environment, variable isolation, cloning | Data model change + full namespace lifecycle |
| Docker Compose support | Parse compose file, create multiple services + networks | Complex parsing + multi-service orchestration |

---

## The Aggregator Question: Can Ink MCP Unify All Platforms?

### The Idea

Instead of competing with Railway/Fly/Render on infrastructure, Ink MCP could be the **single MCP interface** that agents use to deploy to ANY platform. The agent calls `create_service()` and Ink MCP routes to the best backend — your own k8s cluster, Railway, Fly.io, Render, or even raw VPS.

### Why This Could Work

1. **Agents don't care about the platform** — they want `create_service(repo, name)` → URL. The backend is irrelevant to them.
2. **Each platform has an API** — Railway has GraphQL, Fly has Machines REST API, Render has REST, DO has REST. You could write provider adapters.
3. **You already have the MCP surface** — your 12 MCP tools are the agent contract. Adding providers behind them is a backend concern.
4. **Northflank proved BYOC works** — they raised $22M doing exactly this (control plane + multi-cloud runtime). But they target enterprises with k8s expertise. You'd target agents.
5. **Differentiation** — no one else is building "deploy anywhere via MCP." Railway has an MCP server but it only deploys to Railway.

### Architecture Would Look Like

```
Agent → MCP Tools → Provider Router → ┬→ ml.ink k8s (default, cheapest)
                                       ├→ Railway API
                                       ├→ Fly.io Machines API
                                       ├→ Render API
                                       ├→ DigitalOcean API
                                       └→ Raw VPS (SSH + Docker)
```

The `create_service` tool could accept an optional `provider` param, or auto-select based on requirements (need multi-region? → Fly. Need serverless? → Vercel. Need cheap? → ml.ink).

### Why This Is Hard

1. **Lowest common denominator problem** — each platform has different capabilities. Volumes work on Railway but not Vercel. TCP proxy works on Fly but not Render. Your MCP interface would either:
   - Expose the union of all features (complex, many things fail on certain providers)
   - Expose the intersection (too limited)
   - Have provider-specific extensions (leaky abstraction)

2. **Account management** — users need accounts on each platform. You'd need to either:
   - Store their API keys (security liability)
   - OAuth into each platform (massive integration work)
   - Create accounts on their behalf (against most ToS)

3. **Billing aggregation** — users would get billed from 5 different places unless you resell (requires partnerships)

4. **State management** — tracking which service is on which provider, handling migrations between providers, keeping status in sync

5. **Debugging across providers** — logs and metrics come from different systems with different formats

### Realistic Path

**Phase 1 (now):** Build a great experience on your own k8s. This is your moat — you control the infra, the margin, the features.

**Phase 2 (later):** Add ONE external provider for overflow/regions you don't have. Fly.io's Machines API is the most "infrastructure-like" and would integrate cleanest (it's basically `create VM, run image, expose port`).

**Phase 3 (maybe):** True multi-provider routing for specific use cases:
- "I need my app in Asia" → route to Fly.io's Singapore region
- "I need serverless" → route to... your own scale-to-zero, or Lambda
- "I need GPU" → route to CoreWeave/RunPod

The aggregator vision is compelling but premature. **Win on your own infra first, then expand.**

---

## Competitive Positioning

### Where Ink MCP Already Wins

1. **Agent-native interface** — MCP tools designed for AI agents, not humans clicking dashboards
2. **Integrated git hosting** — Gitea means agents don't need a GitHub account
3. **Zero-config deploys** — Railpack auto-detection + sensible defaults
4. **Low cost** — Hetzner bare metal is 5-10x cheaper than cloud providers per compute unit
5. **Fast iteration** — small team, no legacy, ship features daily

### Where Ink MCP Needs To Catch Up

1. **Volumes / stateful workloads** — table stakes, every competitor has this
2. **Health checks** — k8s gives this for free, just wire it up
3. **Managed databases** — at least Postgres, the lingua franca
4. **Custom domains** — users expect to point their domain at their app
5. **Horizontal scaling** — even if it's just `replicas: N`

### What To Ignore (For Now)

1. Multi-region (premature at current scale)
2. PR preview environments (developer feature, agents don't use PRs)
3. Template marketplace (nice-to-have, focus on core)
4. Serverless / scale-to-zero (complex, limited demand from agents)
5. Edge networking (Cloudflare gives you enough)
6. Static outbound IPs (niche)
7. Docker Compose (complexity for marginal gain)

---

## Sources

- [Railway Docs](https://docs.railway.com)
- [Railway V2 Blog — Why they abandoned k8s](https://blog.railway.com/p/railway-v2)
- [Railway Metal — Bare metal infrastructure](https://blog.railway.com/p/data-center-build-part-two)
- [Fly.io Architecture](https://fly.io/docs/reference/architecture/)
- [Fly.io Regions](https://fly.io/docs/reference/regions/)
- [Render — Infrastructure for Scalable AI](https://render.com/articles/infrastructure-for-scalable-ai-beyond-kubernetes)
- [Render $80M Series C](https://www.businesswire.com/news/home/20250121967005/en/Render-Secures-80M-Series-C-Funding-to-Bring-The-Next-Billion-Applications-Online)
- [Vercel + AWS Lambda Architecture](https://vercel.com/blog/aws-and-vercel-accelerating-innovation-with-serverless-computing)
- [Vercel Fluid Compute](https://vercel.com/blog/fluid-how-we-built-serverless-servers)
- [Heroku Architecture — Dynos](https://devcenter.heroku.com/categories/heroku-architecture)
- [DigitalOcean App Platform on Kubernetes](https://thenewstack.io/digitalocean-app-platform-eases-kubernetes-deployments-for-developers/)
- [Coolify — Self-hosted PaaS](https://github.com/coollabsio/coolify)
- [Northflank BYOC](https://northflank.com/features/bring-your-own-cloud)
- [Firecracker microVMs](https://firecracker-microvm.github.io/)
