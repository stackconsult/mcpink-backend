# Deploy MCP

> **One MCP to deploy them all.** Infrastructure for the agentic era.

---

## Motivation

AI agents can now write complete applications. Claude Code, Cursor, and Windsurf generate production-ready code in minutes. But when it's time to deploy, agents hit a wall:

**The Fragmentation Problem:**

- Need Railway MCP for hosting
- Need Neon MCP for database
- Need separate tools for secrets, DNS, SSL
- Manual wiring of connection strings
- Human must create accounts on each platform

**The Result:** Agents build in seconds, but deploying takes hours of human intervention.

```
# Today
Agent: "I've created your SaaS app. Here's the code."
Human: *creates Railway account*
Human: *creates Neon account*
Human: *deploys app manually*
Human: *provisions database*
Human: *copies connection string*
Human: *sets environment variables*
Human: *configures domain*
Human: *waits for SSL*
→ 2 hours later: "It's live"

# With Deploy MCP
Agent: create_service(repo="my-saas", name="my-saas")
→ 60 seconds later: "https://my-saas.ml.ink is live"
```

---

## Vision

**"Internet for Agents"** — Infrastructure that agents can provision autonomously.

Deploy MCP is a **platform**, not just a tool:

- Users authenticate to **us**
- We provision infrastructure using **our** provider credentials
- Agents deploy with **one command**
- Users never touch provider dashboards

---

## Core Principles

| Principle                | Description                                        |
| ------------------------ | -------------------------------------------------- |
| **Repo as Identity**     | `github.com/user/app` is the natural project key   |
| **One Transaction**      | App + database + secrets + domain in a single call |
| **Auto-Deploy Default**  | Push to GitHub → automatic deployment              |
| **Platform Abstraction** | Users never see underlying providers               |
| **Right Tool for Job**   | Frontend → edge, Backend → containers              |

---

## Authentication

Deploy MCP supports two git providers, each with its own auth model:

### Git Providers

| Provider                           | Identity      | Repo Access                             | Webhooks           |
| ---------------------------------- | ------------- | --------------------------------------- | ------------------ |
| **GitHub** (`host=github.com`)     | GitHub OAuth  | GitHub App (installation tokens)        | GitHub App webhook |
| **Internal git** (`host=ml.ink`, default) | Firebase Auth | Per-repo HTTPS tokens (`mlg_...`) via git-server | git-server push hook |

**GitHub flow:** User signs in via GitHub OAuth, installs the GitHub App for repo access, then deploys with `host=github.com`.

**Internal git flow (default):** User signs in via Firebase, gets an auto-provisioned internal git account (petname username like `awake-dassie`). Repos are created via the git-server API with per-repo HTTPS tokens (`mlg_...`). No GitHub account needed.

### MCP Authentication

Agents authenticate to the MCP server (`https://mcp.ml.ink/mcp`) via API key:

```
Authorization: Bearer dk_live_abc123...
```

API keys are hashed with bcrypt (only prefix stored for lookup) and validated on every request.

There are two ways to obtain an API key:

**1. Manual** — Generate from the dashboard (`/settings`)

**2. MCP OAuth** — Automatic via OAuth 2.0 Authorization Code + PKCE (for MCP clients like Claude Desktop):

```
MCP Client                    Product Server              Frontend            User
    │                              │                          │                 │
    │─── GET /.well-known/ ───────▶│                          │                 │
    │◀── auth server metadata ─────│                          │                 │
    │                              │                          │                 │
    │─── POST /oauth/register ────▶│                          │                 │
    │◀── client_id ────────────────│                          │                 │
    │                              │                          │                 │
    │─── GET /oauth/authorize ────▶│                          │                 │
    │    (client_id, redirect_uri, │                          │                 │
    │     code_challenge, state)   │── store in cookie ──────▶│                 │
    │                              │── redirect ─────────────▶│── Firebase ────▶│
    │                              │                          │◀── login ───────│
    │                              │                          │                 │
    │                              │◀── POST /oauth/complete ─│  (consent)      │
    │                              │── generate API key ──────│                 │
    │                              │── generate auth code ────│                 │
    │                              │── return redirect_url ──▶│                 │
    │◀── redirect with code ───────────────────────────────────                 │
    │                              │                                            │
    │─── POST /oauth/token ───────▶│                                            │
    │    (code, code_verifier)     │── verify PKCE                              │
    │◀── access_token (API key) ───│                                            │
```

The OAuth flow creates an API key behind the scenes — the `access_token` returned is a real API key (`dk_live_...`), so both auth methods use the same underlying mechanism.

### Webhooks (Auto-Redeploy)

| Source     | Event                                  | Verification                 |
| ---------- | -------------------------------------- | ---------------------------- |
| GitHub App | `push`, `installation.created/deleted` | HMAC-SHA256 (webhook secret) |
| git-server | `push` (post-receive)                  | Direct Temporal trigger (no webhook) |

Both trigger the same Temporal redeploy workflow with deterministic workflow IDs for deduplication.

---

## Tech Stack

- **Language**: Go
- **MCP Framework**: [mcp-go](https://github.com/modelcontextprotocol/go-sdk)
- **Database**: Postgres (with sqlc)
- **Auth**: Firebase Auth + GitHub OAuth + GitHub App
- **Orchestration**: Temporal (deployment workflows)
- **Container Orchestration**: k3s
- **Build**: BuildKit + Railpack
- **Ingress**: Traefik + Cloudflare LB
- **Git**: Custom git-server (internal, `git.ml.ink`)
- **IaC**: Ansible
- **Compute**: Hetzner (dedicated + cloud)
- **Database Provisioning**: Turso (SQLite)

---

## MCP Tools

| Tool               | Description                                                      | Requirements |
| ------------------ | ---------------------------------------------------------------- | ------------ |
| `whoami`           | Get current user info and GitHub App status                      | API key      |
| `create_service`   | Deploy a service from a git repo (`host=ml.ink` or `github.com`) | API key      |
| `list_services`    | List all deployed services                                       | API key      |
| `get_service`      | Get service details including build/runtime logs                 | API key      |
| `redeploy_service` | Redeploy a service to pull latest code                           | API key      |
| `delete_service`   | Delete a service and its k8s resources                           | API key      |
| `create_resource`  | Provision a database (SQLite via Turso)                          | API key      |
| `list_resources`   | List all provisioned resources                                   | API key      |
| `get_resource`     | Get resource connection details (URL + auth token)               | API key      |
| `delete_resource`  | Delete a resource                                                | API key      |
| `create_repo`      | Create a git repo (`host=ml.ink` default, or `github.com`)       | API key      |
| `get_git_token`    | Get a temporary git token to push code                           | API key      |

### Adding MCP Server to Claude Code

```bash
# Production
claude mcp add --transport http mcpdeploy https://mcp.ml.ink/mcp --header "Authorization: Bearer <your-api-key>"

# Local development
claude mcp add --transport http mcpdeploy http://localhost:8081/mcp --header "Authorization: Bearer <your-api-key>"
```

Webhook ingress remains on `https://api.ml.ink` (`deployer-server` `/healthz` + git webhook receiver).

## Application Binaries

There are 4 binaries split across two runtimes:

- Railway: `cmd/server` + `cmd/worker`
- k3s cluster: `cmd/deployer-server` + `cmd/deployer-worker`

| Binary            | Path                          | Runtime        | Purpose                                                         | Task Queue   | K8s Manifest                    |
| ----------------- | ----------------------------- | -------------- | --------------------------------------------------------------- | ------------ | ------------------------------- |
| `server`          | `cmd/server/main.go`          | Railway        | Product API — GraphQL, MCP server, OAuth, Firebase auth        | —            | —                               |
| `worker`          | `cmd/worker/main.go`          | Railway        | Product Temporal worker — account workflows                     | `default`    | —                               |
| `deployer-server` | `cmd/deployer-server/main.go` | k3s (`dp-system`) | Webhook receiver (GitHub), kicks off Temporal workflows | —            | `infra/eu-central-1/k8s/workloads/deployer-server.yml` |
| `deployer-worker` | `cmd/deployer-worker/main.go` | k3s (`dp-system`) | K8s deployment worker — build, deploy, delete, status          | `deployer-eu-central-1` | `infra/eu-central-1/k8s/workloads/deployer-worker.yml` |

Mapping note: conceptual `k8s-server` = `deployer-server`; conceptual `k8s-worker` = `deployer-worker`.

The deployer-server is **not** the product server. It only handles webhooks and `/healthz` — no GraphQL, no MCP, no OAuth.

---

## Deployment Workflow

When an agent calls `create_service`, the following happens:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           DEPLOYMENT FLOW                                    │
└─────────────────────────────────────────────────────────────────────────────┘

1. AGENT CALLS create_service
   ┌──────────┐     ┌──────────┐     ┌──────────┐
   │  Agent   │────▶│   MCP    │────▶│ Temporal │
   │  (via    │     │  Server  │     │ Workflow │
   │  Claude) │     │          │     │  Start   │
   └──────────┘     └──────────┘     └──────────┘

2. TEMPORAL WORKER (on k3s-1) PICKS UP TASK
   ┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
   │ Create   │────▶│ Clone    │────▶│ Resolve  │────▶│ Build    │
   │ App in   │     │ Repo     │     │ Build    │     │ via      │
   │ Postgres │     │ (HTTPS)  │     │ Pack     │     │ BuildKit │
   └──────────┘     └──────────┘     └──────────┘     └──────────┘
                         │                                  │
                         ▼                                  ▼
                   Mints token (GitHub App       Push image to
                   or internal git token)        internal registry

3. DEPLOY TO K8S
   ┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
   │ kubectl  │────▶│ Create   │────▶│ Wait for │────▶│ Return   │
   │ apply    │     │ NS, Dep, │     │ Rollout  │     │ URL +    │
   │          │     │ Svc, Ing │     │ Ready    │     │ Status   │
   └──────────┘     └──────────┘     └──────────┘     └──────────┘

4. AUTO-REDEPLOY (Push to GitHub or internal git)
   ┌──────────┐     ┌──────────────┐     ┌──────────┐     ┌──────────┐
   │ Git Push │────▶│ GitHub:      │────▶│ Webhook  │────▶│ Temporal │
   │          │     │  webhook     │     │ Handler  │     │ Redeploy │
   └──────────┘     │ git-server:  │     │          │     │ Workflow │
                    │  post-receive│     └──────────┘     └──────────┘
                    └──────────────┘     Deterministic workflow ID
                                        from commit SHA (dedup)
```

### Workflow Idempotency

GitHub webhook delivery is at-least-once, so the same push event may be delivered multiple times. Internal git pushes trigger deploys directly via post-receive hook. The deployment service handles duplicates by:

1. Deriving a deterministic workflow ID from the commit SHA
2. Using Temporal's `REJECT_DUPLICATE` policy
3. Logging and returning success if a workflow for that commit is already running

---

# Architecture

Deploy MCP uses a 4-pool k3s cluster separating control, ops, build, and run concerns.

## Core Philosophy

1. **Abstraction**
   - Agents interact with **Projects** and **Apps** (intent), not "servers" or "containers" (implementation)
   - The MCP surface is stable; providers are replaceable

2. **Safety**
   - User code runs with strong guardrails
   - Drop ALL capabilities, no privilege escalation, no SA token
   - NetworkPolicy blocks private network, k8s API, and metadata endpoints

3. **Workflow Orchestration**
   - Deployments run as Temporal workflows for reliability
   - Automatic retries, idempotency, and observability built-in

---

## Build Packs

Deploy MCP supports multiple build strategies:

| Build Pack           | Use Case                                     |
| -------------------- | -------------------------------------------- |
| `railpack` (default) | Auto-detect language, generate BuildKit plan |
| `dockerfile`         | Custom Dockerfile via BuildKit               |
| `static`             | Static files served by nginx                 |

---

## What Gets Deployed

### Service Types

- **Web apps** — Next.js, Remix, SvelteKit, etc. (SSR or static)
- **APIs** — Express, FastAPI, Go servers
- **Backends** — WebSocket servers, workers, cron jobs

### Database Resources

- **SQLite** — Via Turso (managed, replicated SQLite)

---

## Physical Architecture (4-pool k3s)

We separate control, ops, build, and run across dedicated node pools so CPU-heavy builds never lag a live app.

### Topology

```
┌──────────────────────────────────────────────────────────────┐
│  k3s-1 (ctrl) — k3s Server (Hetzner Cloud CX32)             │
│                                                              │
│  k3s server process (etcd, API server, scheduler)            │
│  Temporal Worker                                             │
│  cert-manager, CoreDNS, metrics-server                       │
│                                                              │
│  Labels: pool=ctrl                                           │
└──────────────────────────────────────────────────────────────┘
         │ private network (Hetzner vSwitch, 10.0.0.0/16)
         │
┌────────┴─────────────────────────────────────────────────────┐
│  ops-1 (ops) — Storage & Observability (Hetzner Dedicated)   │
│                                                              │
│  Docker Registry v2 (NVMe-backed)                            │
│  Loki, Prometheus, Grafana                                   │
│  git-server (custom, git.ml.ink)                             │
│                                                              │
│  Labels: pool=ops    Taint: pool=ops:NoSchedule              │
└──────────────────────────────────────────────────────────────┘
         │
┌────────┴─────────────────────────────────────────────────────┐
│  build-1 (build) — Builder (Hetzner Cloud CCX, dedicated CPU)│
│                                                              │
│  BuildKit daemon (local cache + registry cache)              │
│                                                              │
│  Labels: pool=build    Taint: pool=build:NoSchedule          │
└──────────────────────────────────────────────────────────────┘
         │
┌────────┴─────────────────────────────────────────────────────┐
│  run-1+ (run) — Runners (Hetzner Dedicated)                  │
│                                                              │
│  Traefik (DaemonSet, hostNetwork)                            │
│  Customer containers                                         │
│                                                              │
│  Labels: pool=run                                            │
└──────────────────────────────────────────────────────────────┘
```

### What runs where

**ctrl (k3s-1)** — k3s server, Temporal worker, cert-manager, CoreDNS

**ops (ops-1)** — Docker Registry, git-server, Loki, Prometheus, Grafana

**build (build-1)** — BuildKit daemon with persistent local cache + registry cache

**run (run-1+)** — Traefik DaemonSet (ingress), customer containers

### Networking

- **Private network**: Hetzner vSwitch (`10.0.0.0/16`) bridges cloud and dedicated servers
- **Public ingress source of truth**: Cloudflare LB (`*.ml.ink`) → run node origin pool → Traefik → k8s Service
- **TLS**: Wildcard Let's Encrypt cert via cert-manager DNS-01, served by Traefik TLSStore

---

## MCP Interface (The Agent Contract)

### Principles

- **Name-based** — Agents reference services by name, not IDs
- **Discoverable** — `list_*`, `get_*` tools for exploration
- **Logs are first-class** — Agents can self-debug via `get_service`

### Tool Signatures

#### Services

```
create_service(repo, host?, branch?, name, project?, build_pack?, port?, env_vars?, memory?, cpu?, install_command?, build_command?, start_command?)
list_services()
get_service(name, project?, include_env?, deploy_log_lines?, runtime_log_lines?)
redeploy_service(name, project?)
delete_service(name, project?)
```

#### Resources (Databases)

```
create_resource(name, type?, size?, region?)
list_resources()
get_resource(name)
delete_resource(name)
```

#### Git

```
create_repo(name, host?, description?)
get_git_token(name, host?)
```

#### Identity

```
whoami()
```

---

## Databases

### Current Implementation

**SQLite via Turso** — Managed, replicated SQLite databases.

```
create_resource(name="my-db", type="sqlite", region="eu-west")
```

Returns:

- `url` — libSQL connection URL
- `auth_token` — Authentication token (encrypted at rest)

### Future Options

- **Postgres** — Via Neon or self-hosted
- **Redis/KV** — Via Upstash for caching/queues
- **Bring-your-own** — Connection string passthrough

---

## Container Registry

Internal Docker Registry v2 running on ops-1 (NVMe-backed), accessible only on the private network (`10.0.1.4:5000`). Host firewall blocks port 5000 from the public internet.

A nightly GC CronJob keeps the last 2 tags per service via `registry garbage-collect --delete-untagged`.

Images are treated as cache/artifacts, not the source of truth — they can always be rebuilt from source.

---

## Ops Manual

### Adding Run Nodes

New run nodes are provisioned via Ansible:

```bash
# 1. Buy Hetzner Auction server, attach to Robot vSwitch
# 2. Add to inventory under run.hosts in infra/ansible/inventory/hosts.yml
# 3. Run the playbook:
ansible-playbook playbooks/add-run-node.yml --limit run-2
# 4. Add run-2 public IP to Cloudflare origin pool (Traffic → Load Balancing → run-nodes pool)
```

The playbook applies: common hardening, vSwitch networking, k3s agent join, registry client config, and firewall rules.

### Cloudflare LB Pool Management

Cloudflare LB is the source of truth for public ingress (`*.ml.ink` → run-node origin pool). To add or remove run nodes, update the `run-nodes` origin pool in the Cloudflare dashboard (Traffic → Load Balancing → Pools). No DNS records to manage.

### Cluster Management

Infrastructure is managed via Ansible playbooks in `infra/ansible/`:

- `site.yml` — Full cluster setup
- `add-run-node.yml` — Add a new run node
- `upgrade-k3s.yml` — Rolling k3s upgrade

K8s manifests live in `infra/eu-central-1/k8s/` and are applied by Ansible.

---

## Backups & Restore

### What we back up

1. **MCP State (critical)** — Postgres on Supabase (projects, resources, users, API keys). Supabase handles backups.

2. **User Data (critical)** — Turso databases. Provider-native backups.

3. **Internal Git Repos (important)** — Hosted on ops-1 (`/mnt/git-repos`) with NVMe RAID1 for built-in redundancy.

4. **Registry Images (rebuildable)** — Treated as cache/artifacts. Can always be rebuilt from source.

### Restore procedure (disaster recovery)

**Scenario: ctrl node (k3s-1) lost**

1. Provision a fresh Cloud server
2. Run `ansible-playbook site.yml --limit k3s-1`
3. Restore etcd from backup or re-apply k8s manifests
4. Redeploy Temporal worker

**Downtime reality check**

- Existing apps on run nodes continue serving traffic — Traefik runs independently on each run node
- You lose deploy/management capability until the ctrl node is restored

---

## Security Configuration

### Pod Security

Customer pods run with defense-in-depth isolation:

| Layer                  | Protection                                                                                                                      |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| **Container security** | Drop ALL capabilities, `allowPrivilegeEscalation: false`, `automountServiceAccountToken: false`                                 |
| **Network ingress**    | NetworkPolicy: default-deny, allow only same-namespace + Traefik                                                                |
| **Network egress**     | NetworkPolicy: allow DNS + public internet, block `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, metadata (`169.254.169.254`) |
| **Registry**           | Host firewall: port 5000 only from `10.0.0.0/16`                                                                                |
| **Quotas**             | Per-namespace ResourceQuota reconciled by infra (Ansible/systemd on ctrl): defaults 40 CPU limits, 40 CPU requests, 40Gi memory limits/requests, 200 pods |

### Host-Level Hardening

See `infra/ansible/roles/firewall/` for iptables rules blocking metadata endpoints and restricting registry access to the private network.

---

## Non-goals (for sanity)

- Replacing GitHub (we integrate with it alongside our internal git)
- Building a full PaaS UI for users (MCP is the interface)
- Solving arbitrary sandboxing perfectly on day 1 (ship baseline + iterate)

---

## Design Notes

### 4-Pool Architecture

The separation of ctrl, ops, build, and run pools ensures CPU-heavy builds never compete with customer workloads or infrastructure services.

### Temporal Workflows

All deployments run as Temporal workflows, providing:

- Automatic retries on transient failures
- Idempotency for webhook-triggered deploys (deterministic workflow ID from commit SHA)
- Visibility into deployment progress
- Clean separation of orchestration from business logic
- No secrets in workflow history — tokens minted inside activities from k8s Secrets

---

## References

[1]: https://docs.hetzner.com/networking/networks/connect-dedi-vswitch/ "Connect Dedicated Servers (vSwitch)"
[2]: https://docs.hetzner.com/robot/dedicated-server/network/vswitch/ "vSwitch"
