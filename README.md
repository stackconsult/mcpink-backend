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
Agent: deploy(repo="github.com/user/my-saas", database={type:"postgres"})
→ 60 seconds later: "https://my-saas.deploy.app is live"
```

---

## Vision

**"Internet for Agents"** — Infrastructure that agents can provision autonomously.

Deploy MCP is a **platform**, not just a tool:
- Users authenticate to **us** (not Fly.io, not Neon)
- We provision infrastructure using **our** provider credentials
- Agents deploy with **one command**
- Users never touch provider dashboards

---

## Core Principles

| Principle | Description |
|-----------|-------------|
| **Repo as Identity** | `github.com/user/app` is the natural project key |
| **One Transaction** | App + database + secrets + domain in a single call |
| **Auto-Deploy Default** | Push to GitHub → automatic deployment |
| **Platform Abstraction** | Users never see Railway/Fly/Cloudflare |
| **Right Tool for Job** | Frontend → edge, Backend → containers |
| **Future-Proof** | Abstract now, own infrastructure later |

---

## Architecture

### Platform Model

```
┌─────────────────────────────────────────────────────────────────────┐
│                              USERS                                   │
│                                                                      │
│         Auth: GitHub OAuth → API Key                                 │
│         See: Deploy MCP only                                         │
│         Never see: Fly.io, Neon, Cloudflare credentials              │
└───────────────────────────────┬─────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                        DEPLOY MCP PLATFORM                           │
│                                                                      │
│   ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────────┐ │
│   │   Users &   │  │  Projects   │  │  Usage Tracking / Billing   │ │
│   │   API Keys  │  │  & Deploys  │  │                             │ │
│   └─────────────┘  └─────────────┘  └─────────────────────────────┘ │
│                                                                      │
│   ┌──────────────────────────────────────────────────────────────┐  │
│   │                 PROVIDER INTERFACE LAYER                      │  │
│   │                                                               │  │
│   │  BackendProvider   FrontendProvider   DatabaseProvider        │  │
│   │  SecretsProvider   DNSProvider        StorageProvider         │  │
│   └──────────────────────────────────────────────────────────────┘  │
│                                                                      │
│                    OUR credentials, OUR accounts                     │
└───────────────────────────────┬─────────────────────────────────────┘
                                │
        ┌───────────────────────┼───────────────────────┐
        ▼                       ▼                       ▼
   ┌─────────┐            ┌─────────┐            ┌─────────┐
   │ Phase 1 │            │ Phase 2 │            │ Phase 3 │
   │  (MVP)  │            │ (Scale) │            │ (Margin)│
   │         │            │         │            │         │
   │ Fly.io  │            │ +Multi  │            │ Coolify │
   │ CF Pages│            │ region  │            │ Hetzner │
   │ Neon    │            │ +Railway│            │         │
   └─────────┘            └─────────┘            └─────────┘
```

---

## Authentication

Users authenticate to Deploy MCP. We handle all provider credentials internally.

### Flow

```
1. User visits deploy-mcp.dev
2. "Sign in with GitHub" → OAuth
3. We store: user identity + GitHub token (for private repos)
4. Dashboard shows API key: dk_live_abc123...
5. User adds to MCP config:

{
  "mcpServers": {
    "deploy": {
      "command": "deploy-mcp",
      "env": {
        "DEPLOY_API_KEY": "dk_live_abc123..."
      }
    }
  }
}

6. All MCP calls authenticated via API key
7. We use OUR provider credentials behind the scenes
```

### Why GitHub OAuth?

- **Repo is the project key** — Need GitHub identity anyway
- **Private repo access** — OAuth token lets us clone user's repos
- **Verify ownership** — Confirm user owns repo before deploying
- **Familiar** — Every developer has GitHub

---

## Provider Analysis

### Phase 1 Candidates (MVP)

| Provider | Strengths | Weaknesses | API | Verdict |
|----------|-----------|------------|-----|---------|
| **Fly.io** | **Owns hardware** (better margins), global edge, subsecond VM starts, multi-region built-in, excellent REST API, official Go SDK | Requires Docker image (no buildpacks in API) | REST (`api.machines.dev`) | ✅ **Best for MVP** |
| **Railway** | Simple GraphQL API, Nixpacks auto-detection, fast DX, visual canvas | Runs on GCP (2-5x markup), single region default | GraphQL v2 | Good alternative |
| **Render** | Predictable pricing, render.yaml IaC, managed Postgres | Runs on AWS/GCP (markup), per-seat pricing | REST | Fallback option |

**Decision: Fly.io for Phase 1**
- **Owns their hardware** → Better unit economics from day 1
- **REST API** at `api.machines.dev` with OpenAPI spec
- **Official Go SDK** (`superfly/fly-go`)
- **Multi-region by default** → Better for users
- **Subsecond VM starts** → Great cold start experience

### Fly.io Machines API

```bash
# Base URL
https://api.machines.dev

# Create app
POST /v1/apps
{ "app_name": "my-app", "org_slug": "personal" }

# Create machine (deploy)
POST /v1/apps/{app_name}/machines
{
  "config": {
    "image": "registry.fly.io/my-app:latest",
    "env": { "DATABASE_URL": "postgres://..." },
    "services": [{ "ports": [{ "port": 443 }], "protocol": "tcp" }],
    "guest": { "cpu_kind": "shared", "cpus": 1, "memory_mb": 256 }
  },
  "region": "ord"
}

# Machine lifecycle
POST /v1/apps/{app_name}/machines/{id}/start
POST /v1/apps/{app_name}/machines/{id}/stop
DELETE /v1/apps/{app_name}/machines/{id}

# Secrets (via flyctl or GraphQL)
fly secrets set DATABASE_URL=postgres://...
```

### Frontend Provider

| Provider | Strengths | Weaknesses | Verdict |
|----------|-----------|------------|---------|
| **Cloudflare Pages** | Free, global edge, instant deploys, excellent API, built-in GitHub integration | Limited to static/SSR | ✅ **Use this** |
| **Vercel** | Great Next.js support, previews | Expensive at scale, serverless limits | Too expensive |
| **Netlify** | Good free tier, functions | Limited backend | CF Pages better |

**Decision: Cloudflare Pages**
- Free (yes, actually free)
- Global edge by default
- Great API

### Database Provider

| Provider | Strengths | Weaknesses | Verdict |
|----------|-----------|------------|---------|
| **Neon** | Instant provisioning, branching, scale-to-zero, excellent API | Postgres only | ✅ **Use this** |
| **PlanetScale** | MySQL, branching | MySQL only, recent pricing changes | Phase 2 |
| **Supabase** | Postgres + Auth + Storage | Heavier than needed for just DB | Phase 2 |

**Decision: Neon**
- Used by Replit, Bolt.new (proven at scale)
- Branching = perfect for preview environments
- Scale-to-zero = cost efficient

### Phase 1 Stack

| Component | Provider | Cost Model | Why |
|-----------|----------|------------|-----|
| **Backend** | Fly.io | Usage-based (~$2-15/mo per app) | Owns hardware, REST API, multi-region |
| **Frontend** | Cloudflare Pages | Free | Global edge, instant deploys |
| **Database** | Neon | Free tier, then usage-based | Instant provisioning, branching |
| **DNS/SSL** | Cloudflare | Free | Already using for Pages |

---

## Phase 3: Own Infrastructure (Further Optimization)

### The Economics with Fly.io

Unlike Railway/Render (which run on AWS/GCP), **Fly.io owns their hardware**. This means:
- Better unit economics from day 1
- No cloud provider markup passed to us
- **Phase 1 margins already ~40-50%** (vs. 20-30% with Railway)

### Phase 3: Full Control

For maximum margins and control, we can run our own infrastructure:

**Coolify** is an open-source, self-hosted PaaS:
- 44,000+ GitHub stars
- Full API for programmatic deployments
- Git integration, auto-SSL, webhooks
- Supports any Docker container

**Hetzner** is a German cloud provider:
- VPS from €3.79/mo ($4)
- Dedicated servers from €39/mo
- 10x cheaper than AWS/GCP

### Phase 3 Stack

| Component | Provider | Cost |
|-----------|----------|------|
| Backend | Coolify on Hetzner | ~$4-10/mo per server (many apps) |
| Frontend | Cloudflare Pages | Free (keep this) |
| Database | Own Postgres or Neon | ~$10-50/mo |
| DNS/SSL | Cloudflare | Free |

**Our margin on $20/mo app: ~70-80%** (up from ~40-50% with Fly.io)

### Migration Path

```
Phase 1: Fly.io Machines API → our BackendProvider interface
Phase 3: Coolify API         → same BackendProvider interface

User-facing API: UNCHANGED
```

The provider abstraction means zero breaking changes when we migrate.

---

## Sequencing

### Phase 1: MVP (Weeks 1-6)

**Goal:** Validate product-market fit

```
Week 1-2: Core Infrastructure
├── User auth (GitHub OAuth)
├── API key generation
├── Project CRUD (repo as key)
└── Basic dashboard

Week 3-4: Deployment
├── Fly.io integration (backend) via Machines API
├── Cloudflare Pages integration (frontend)
├── Auto-detection (frontend vs backend)
├── Docker image building (for Fly.io)
└── GitHub webhook setup (auto-deploy)

Week 5-6: Database & Polish
├── Neon integration (Postgres)
├── Secrets management
├── Custom domains + SSL
└── Logs and status
```

**MVP Features:**
- [x] GitHub OAuth + API keys
- [x] Project CRUD (repo as key)
- [x] Frontend → Cloudflare Pages
- [x] Backend → Fly.io Machines
- [x] Postgres → Neon
- [x] Auto-deploy via webhooks
- [x] Secrets (project + env scope)
- [x] Custom domains + SSL
- [x] Logs and status

**MVP Non-Features:**
- MySQL, Redis
- Preview deployments
- Rollbacks
- Teams
- Billing

### Phase 2: Scale (Weeks 7-12)

**Goal:** Multi-region and advanced features

- [ ] Multi-region deployment (Fly.io supports this natively)
- [ ] Add Railway as alternative backend option
- [ ] MySQL, Redis support
- [ ] Preview deployments for PRs
- [ ] Blue-green / canary deployments
- [ ] Usage analytics
- [ ] Billing integration

### Phase 3: Own Infrastructure (Months 4-6)

**Goal:** Maximum margins

- [ ] Deploy Coolify on Hetzner
- [ ] Implement Coolify provider (same interface)
- [ ] Migrate workloads incrementally
- [ ] Own Postgres clusters (or keep Neon)
- [ ] Volume discounts with Cloudflare

---

## Provider Interfaces

All providers implement standard Go interfaces. Swap implementations without changing MCP tools.

```go
// BackendProvider deploys containerized services
type BackendProvider interface {
    Deploy(ctx context.Context, req DeployRequest) (*Deployment, error)
    GetDeployment(ctx context.Context, id string) (*Deployment, error)
    GetLogs(ctx context.Context, id string, opts LogOptions) (io.Reader, error)
    SetEnvVars(ctx context.Context, id string, vars map[string]string) error
    Scale(ctx context.Context, id string, replicas int) error
    Destroy(ctx context.Context, id string) error
    SetupWebhook(ctx context.Context, id, repo, branch string) error
}

// FrontendProvider deploys static/SSR to edge
type FrontendProvider interface {
    Deploy(ctx context.Context, req FrontendDeployRequest) (*Deployment, error)
    GetDeployment(ctx context.Context, id string) (*Deployment, error)
    SetEnvVars(ctx context.Context, id string, vars map[string]string) error
    ConnectRepo(ctx context.Context, id, repo, branch string) error
    Destroy(ctx context.Context, id string) error
}

// DatabaseProvider manages database instances
type DatabaseProvider interface {
    Create(ctx context.Context, req DatabaseRequest) (*Database, error)
    Get(ctx context.Context, id string) (*Database, error)
    GetConnectionString(ctx context.Context, id string) (string, error)
    Delete(ctx context.Context, id string) error
}

// DNSProvider manages domains and SSL
type DNSProvider interface {
    AddRecord(ctx context.Context, domain string, record DNSRecord) error
    VerifyDomain(ctx context.Context, domain string) (*VerifyResult, error)
    ProvisionSSL(ctx context.Context, domain string) error
}
```

### Phase 1 Implementations

```go
// flyio.go
type FlyProvider struct {
    client  *http.Client
    token   string
    baseURL string  // https://api.machines.dev
}

func (f *FlyProvider) Deploy(ctx context.Context, req DeployRequest) (*Deployment, error) {
    // 1. Create app if not exists
    // POST /v1/apps { "app_name": appName, "org_slug": "personal" }
    
    // 2. Create machine with image
    // POST /v1/apps/{app}/machines
    // { "config": { "image": "...", "env": {...}, "guest": {...} }, "region": "ord" }
    
    // 3. Allocate IP for public access
    // (via GraphQL API or flyctl)
}

// cloudflare_pages.go
type CloudflarePagesProvider struct {
    client *cloudflare.Client
}

// neon.go
type NeonProvider struct {
    client *neon.Client
}
```

### Fly.io SDK Option

```go
import "github.com/superfly/fly-go"

// Official Go SDK available
client := fly.NewClient(token)
app, err := client.CreateApp(ctx, fly.CreateAppInput{
    Name: "my-app",
    Org:  "personal",
})
```

### Phase 3 Implementation

```go
// coolify.go
type CoolifyProvider struct {
    client *coolify.Client
    servers []Server  // Our Hetzner servers
}

func (c *CoolifyProvider) Deploy(ctx context.Context, req DeployRequest) (*Deployment, error) {
    // Same interface, different implementation
    // Call Coolify API instead of Railway
}
```

---

## MCP Tools

### Core Tools (MVP)

```
deploy(
  repo: string                     # "github.com/user/my-app"
  env?: string                     # "dev" | "staging" | "prod"
  branch?: string                  # Default: "main"
  auto_deploy?: boolean            # Default: true
  database?: { type: "postgres" }  # Provision with deployment
  secrets?: { [key]: value }       # Set secrets
  domain?: string                  # Custom domain
) → {
  deployment_id, url, status,
  auto_deploy: { enabled, branch },
  database?: { id, connection_string },
  domain?: { domain, ssl_status, dns_records }
}

provision_database(repo, type) → { id, connection_string }

set_secret(repo, key, value, env?) → { key, scope }

get_secrets(repo, env?) → { secrets[] }  # Values NEVER returned

setup_domain(repo, domain, env?) → { domain, ssl_status, dns_records }

get_logs(repo, env?, lines?) → { logs }

get_status(repo, env?) → { deployment, database?, domain? }

redeploy(repo, env?) → { deployment_id, url }

destroy_deployment(repo, env, confirm: true) → { deleted }
```

### Example: Full Deployment

```
# Agent deploys a SaaS app

1. deploy(
     repo: "github.com/alice/my-saas",
     database: { type: "postgres" },
     secrets: { "STRIPE_KEY": "sk_live_..." }
   )
   
   → { 
       url: "https://my-saas-dev.deploy.app",
       database: { connection_string: "postgres://..." },
       auto_deploy: { enabled: true, branch: "main" }
     }

2. # User pushes code → auto-deploys (no agent needed)

3. setup_domain(
     repo: "github.com/alice/my-saas",
     domain: "my-saas.com",
     env: "prod"
   )
   
   → { dns_records: [{ type: "CNAME", name: "@", value: "..." }] }

# Done. Production deployment in 2 tool calls.
```

---

## Data Model

```
User (GitHub identity)
│
└── Project (keyed by repo URL)
    │
    ├── type: "frontend" | "backend" | "fullstack"
    │
    ├── Environments
    │   └── dev, staging, prod
    │       └── Deployment
    │           ├── url
    │           ├── status
    │           ├── provider_id (internal)
    │           └── auto_deploy config
    │
    ├── Database (optional)
    │   ├── type: postgres
    │   └── connection → injected as DATABASE_URL
    │
    ├── Secrets
    │   ├── Project-level (all envs)
    │   └── Environment-level (overrides)
    │
    └── Domains
        └── domain → environment mapping
```

---

## Project Type Detection

| Files | Type | Deploy To |
|-------|------|-----------|
| `vite.config.*`, `next.config.*` (static export) | frontend | Cloudflare Pages |
| Static HTML/CSS/JS only | frontend | Cloudflare Pages |
| `go.mod` + `main.go` | backend | Fly.io |
| `requirements.txt`, `pyproject.toml` | backend | Fly.io |
| `package.json` with server entry | backend | Fly.io |
| `Dockerfile` | backend | Fly.io |
| Next.js with API routes | fullstack | Fly.io (SSR) |

### Docker Image Building

Fly.io Machines API requires a Docker image. For repos without Dockerfile:

1. **Detect language** (Go, Python, Node, etc.)
2. **Generate Dockerfile** or use buildpacks
3. **Build image** using Fly's remote builder or our own
4. **Push to registry** (Fly registry or Docker Hub)
5. **Deploy machine** with image reference

---

## Security

| Principle | Implementation |
|-----------|----------------|
| User credentials isolated | Users never see provider API keys |
| Secrets encrypted | At rest (AES-256) and in transit (TLS) |
| Secrets never returned | `get_secrets()` returns keys only |
| GitHub token scoped | Repo access only, not account admin |
| Audit logging | All operations logged |
| Webhook verification | HMAC signatures |

---

## Success Metrics

| Metric | Target |
|--------|--------|
| Time to first deploy | < 60 seconds |
| Zero-config success rate | > 90% of repos |
| Auto-deploy latency | < 30 seconds from push |
| Deployment success rate | > 99% |

---

## Competitive Positioning

| vs. | Their Approach | Our Approach |
|-----|----------------|--------------|
| **Fly.io MCP** | Deploy to Fly.io only, user needs account | User doesn't need Fly account, we abstract it |
| **Railway MCP** | Deploy to Railway only | Multi-provider, better margins with Fly.io |
| **Vercel** | Frontend-focused, serverless | Full stack, containers, databases |
| **Neon MCP** | Database only | Full deployment stack |
| **Pulumi Neo** | Full IaC agent | Simpler, no IaC learning curve |

---

## Open Questions

1. **Pricing**: Per-deployment? Per-project? Usage passthrough + margin?
2. **Free tier**: Credits or always-free for hobby?
3. **Fullstack**: Split (CF Pages + Fly.io) or unified (all Fly.io)?
4. **Preview environments**: PR-based deploys in MVP or Phase 2?
5. **Image building**: Use Fly's remote builder, Docker Hub, or self-hosted?

---

## Tech Stack (Implementation)

- **Language**: Go
- **MCP Framework**: mcp-go
- **Database**: SQLite (MVP) → Postgres (scale)
- **Auth**: GitHub OAuth + JWT