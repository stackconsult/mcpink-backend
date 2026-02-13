# Agent-First Deployment MCP: Feature Strategy

> Agents deploy 100x more apps than humans. Design every feature for the agent first, human second.
> This document was written without knowledge of existing infrastructure.

---

## Table of Contents

1. [Agent vs Human: What's Different](#agent-vs-human-whats-different)
2. [Core Principles](#core-principles)
3. [Tier 1: Ship Now (Foundation)](#tier-1-ship-now)
4. [Tier 2: Ship Soon (Competitive Edge)](#tier-2-ship-soon)
5. [Tier 3: Ship Later (Moat)](#tier-3-ship-later)
6. [Tool Design Guidelines](#tool-design-guidelines)
7. [Error Design](#error-design)
8. [The Full Tool Surface](#the-full-tool-surface)

---

## Agent vs Human: What's Different

| Dimension          | Human developer                                     | AI agent                                                         |
| ------------------ | --------------------------------------------------- | ---------------------------------------------------------------- |
| **Debugging**      | Opens dashboard, reads logs visually, clicks around | Can ONLY see what tool responses return                          |
| **Error recovery** | Googles error, reads docs, tries something          | Retries with modified params based on error message              |
| **Speed**          | One deploy per session                              | 10+ deploys in a single conversation                             |
| **Context**        | Remembers past sessions, bookmarks URLs             | Stateless — every conversation starts fresh                      |
| **Accounts**       | Has GitHub, Railway, AWS logins                     | Has ONE API key to your MCP                                      |
| **Patience**       | Will wait 5 minutes for a build                     | Every second of waiting = tokens burned and context consumed     |
| **Wiring**         | Manually copies connection strings between services | Needs env vars auto-injected or returned in tool responses       |
| **Validation**     | Reviews code before deploying                       | May deploy broken code, needs fast feedback to iterate           |
| **Trust model**    | Reads docs, understands limits                      | Discovers capabilities from tool descriptions and error messages |
| **Scale**          | One project at a time                               | May manage dozens of services for one user                       |

**The key insight:** Agents live in a tool-response-only world. If information isn't in the tool response, it doesn't exist. If an error doesn't explain what to do differently, the agent will repeat the mistake. Every feature must be designed for this constraint.

---

## Core Principles

### 1. One Transaction, Full Stack

The single most important feature. An agent should go from "I built a Next.js app with a database" to "it's live" in one or two tool calls, not ten.

```
# BAD: Agent has to orchestrate 6 calls and wire everything manually
create_repo(name="my-app")
token = get_git_token(name="my-app")
# ... push code ...
db = create_resource(name="my-db", type="sqlite")
create_service(repo="my-app", name="my-app")
# Now agent has to figure out env var wiring
set_env_vars(name="my-app", vars={"DATABASE_URL": db.url, "DATABASE_TOKEN": db.auth_token})
redeploy_service(name="my-app")

# GOOD: One call, everything wired
create_service(
  repo="my-app",
  name="my-app",
  resources=[{type: "sqlite", name: "my-db"}],  # auto-provisions + auto-injects env vars
  volumes=[{mount_path: "/data", size: "10Gi"}]
)
# → Returns URL, database URL already in env, volume mounted, webhook set up
```

### 2. Errors Are Documentation

Every error response must tell the agent exactly what went wrong and how to fix it. Agents can't Google things.

```json
// BAD
{"error": "invalid request"}

// GOOD
{
  "error": "port_conflict",
  "message": "Port 3000 is already used by service 'api-server' in this project.",
  "suggestion": "Use a different port (e.g., port=3001) or a different service name.",
  "docs_hint": "Each service in a project must use a unique port."
}
```

### 3. Explicit State Over Silent Idempotency

Create operations should **fail with full context**, not silently succeed. Idempotent creates are ambiguous — the agent doesn't know if it just created something or found something pre-existing, and the behavior when config differs (update? overwrite? ignore?) is always surprising.

```json
// Agent calls create_service("my-app") when it already exists:
{
  "error": "service_already_exists",
  "message": "Service 'my-app' already exists.",
  "existing_service": {
    "name": "my-app",
    "status": "running",
    "url": "https://my-app.ml.ink",
    "memory": "512Mi",
    "volumes": [{ "mount_path": "/data", "size": "10Gi" }]
  },
  "suggestion": "Use update_service(name='my-app', ...) to modify, or delete_service(name='my-app') to recreate."
}
```

The agent now has exact state and can make an informed decision: update, delete+recreate, or move on.

**The exception is delete** — `delete_service("my-app")` on something already deleted should return success, not 404. The desired end state (service gone) is already achieved and there's no ambiguity.

| Operation                    | Behavior on duplicate                         |
| ---------------------------- | --------------------------------------------- |
| `create_service("my-app")`   | Error + return existing service state         |
| `create_resource("my-db")`   | Error + return existing connection info       |
| `create_repo("my-app")`      | Error + return existing repo URL + token      |
| `delete_service("my-app")`   | Success (idempotent — desired state achieved) |
| `redeploy_service("my-app")` | Always succeeds — triggers a new deploy       |

### 4. Self-Describing Responses

Every tool response should include enough context for the agent to take the next action without calling another tool.

```json
// BAD: Agent has to call get_service to learn the URL
{"status": "deployed", "service_id": "svc_123"}

// GOOD: Agent has everything it needs
{
  "status": "deployed",
  "name": "my-app",
  "url": "https://my-app.ml.ink",
  "resources": [
    {"name": "my-db", "type": "sqlite", "env_var": "DATABASE_URL"}
  ],
  "volumes": [
    {"mount_path": "/data", "size": "10Gi"}
  ],
  "build": {"status": "success", "duration_seconds": 34},
  "next_steps": ["Push code to trigger auto-redeploy", "Access logs via get_service(name='my-app', runtime_log_lines=50)"]
}
```

### 5. Minimal Tool Count, Maximum Capability

Every tool in the MCP description consumes tokens. MCP clients like Cursor have tool limits (Cursor: 40 tools). Keep the tool surface small and each tool powerful.

---

## Tier 1: Ship Now

These features make agents choose your platform over manually wiring Railway + Neon + Cloudflare.

### 1.1 Single-Command Deploy with Auto-Wiring

**The killer feature.** Agent creates code, pushes to repo, deploys — one call.

```
create_service(
  repo="my-saas",
  name="my-saas",
  # Auto-detection
  build_pack="auto",           # Railpack auto-detects language + framework
  port=0,                       # Auto-detect from framework (Next.js=3000, Go=8080, etc.)

  # Resources (auto-provisioned + auto-injected)
  resources=[
    {type: "sqlite", name: "main-db"}
  ],

  # Volumes (auto-mounted)
  volumes=[
    {mount_path: "/data", size: "10Gi"}
  ],

  # Env vars (merged with auto-injected resource vars)
  env_vars={
    "NODE_ENV": "production"
  }
)
```

Response includes everything:

```json
{
  "name": "my-saas",
  "url": "https://my-saas.ml.ink",
  "status": "deploying",
  "build_pack": "nodejs-nextjs",
  "port": 3000,
  "resources": [
    {
      "name": "main-db",
      "type": "sqlite",
      "env_vars_injected": {
        "DATABASE_URL": "libsql://main-db-user.turso.io",
        "DATABASE_AUTH_TOKEN": "eyJhbGciOiJFZERTQSIs..."
      }
    }
  ],
  "volumes": [{ "mount_path": "/data", "size": "10Gi", "replicas": 2 }],
  "auto_deploy": true,
  "webhook_active": true
}
```

### 1.2 Create Repo + Push: The Irreducible 2-Step Flow

Agents generate code locally. Getting it deployed requires git — there's no shortcut. MCP is a pure HTTP/OAuth protocol where tool arguments flow through the LLM's context window, so accepting tarballs or file uploads directly is impractical (even a small app would be millions of tokens as base64). Git is the right transport for code.

The minimum flow is 2 steps:

```
# Step 1: Create service (auto-creates repo if it doesn't exist, returns git credentials)
create_service(repo="my-app", name="my-app")
# → Returns: url, clone_url, git_token, status="awaiting_first_push"

# Step 2: Agent pushes code using git (happens outside MCP, in the agent's terminal)
# git remote add origin <clone_url> && git push

# Webhook auto-triggers build + deploy. No Step 3 needed.
```

The key optimization: `create_service` should auto-create the Gitea repo when it doesn't exist and return the `git_token` + `clone_url` inline. This saves the agent from calling `create_repo` and `get_git_token` separately. One MCP call + one git push = deployed.

For GitHub repos, the flow is even simpler — the repo already exists, the agent just calls `create_service(repo="my-app", host="github.com")` and the webhook handles everything on push.

### 1.3 Rich Logs in get_service

Agents debug by reading logs. Make logs a first-class tool output, not an afterthought.

```
get_service(
  name="my-app",
  deploy_log_lines=100,     # Last N lines of build/deploy output
  runtime_log_lines=50,     # Last N lines of application stdout/stderr
  include_env=true           # Show current env vars (full values, not redacted)
)
```

Response:

```json
{
  "name": "my-app",
  "status": "running",
  "url": "https://my-app.ml.ink",
  "deploy_log": {
    "lines": ["Step 1/12: FROM node:20...", "..."],
    "status": "success",
    "duration_seconds": 45,
    "started_at": "2025-02-12T10:00:00Z"
  },
  "runtime_log": {
    "lines": [
      "Server started on port 3000",
      "Error: ECONNREFUSED 127.0.0.1:5432"
    ],
    "crash_detected": true,
    "restart_count": 3,
    "last_exit_code": 1
  },
  "env_vars": {
    "NODE_ENV": "production",
    "DATABASE_URL": "libsql://main-db-user.turso.io",
    "DATABASE_AUTH_TOKEN": "eyJhbGciOiJFZERTQSIs..."
  }
}
```

**Why full values, not redacted:** Agents are stateless across conversations. A new agent session debugging a crash needs to see the actual `DATABASE_URL` to verify it's correct. If the agent set the secrets, it already knows them. If a different agent session picks up the work, it needs them to wire services or diagnose connection failures. The security boundary is the API key — if someone has a valid key, they own that account's infrastructure. Redacting secrets from the owner is security theater.

**Key fields for agents:**

- `crash_detected` — agent immediately knows the app is broken
- `restart_count` — agent knows if it's a persistent crash, not a transient error
- `last_exit_code` — agent can diagnose the class of failure
- `runtime_log.lines` — agent reads the actual error to fix it

### 1.4 Structured Health Status

```json
{
  "health": {
    "status": "unhealthy",
    "reason": "crash_loop",
    "message": "Container exited 3 times in the last 5 minutes with code 1",
    "last_error_line": "Error: Cannot find module 'express'",
    "suggestion": "The app is missing a dependency. Check that 'express' is in package.json dependencies (not devDependencies) and that install_command runs 'npm install'."
  }
}
```

### 1.5 Explicit State on Duplicate Operations

Create operations fail with full context so the agent knows exactly what exists. This is better than silent idempotency because the agent can compare existing state to desired state and decide whether to update, recreate, or move on.

| Operation                    | On duplicate                                                          |
| ---------------------------- | --------------------------------------------------------------------- |
| `create_service("my-app")`   | Error + full existing service state (config, URL, volumes, resources) |
| `create_resource("my-db")`   | Error + existing connection info                                      |
| `create_repo("my-app")`      | Error + existing repo URL + token                                     |
| `delete_service("my-app")`   | Success — idempotent (desired state is "gone", already gone)          |
| `redeploy_service("my-app")` | Always succeeds — triggers new deploy                                 |

---

## Tier 2: Ship Soon

Features that make agents significantly more effective and make your platform sticky.

### 2.1 Service Linking and Dependency Graphs

Agents build multi-service architectures. They need to wire services together.

```
# Agent deploys an API
create_service(name="api", repo="my-api", port=8080)

# Agent deploys a frontend that talks to the API
create_service(
  name="frontend",
  repo="my-frontend",
  env_vars={
    "API_URL": "{{service:api:internal_url}}"   # Template reference
  }
)
```

The template `{{service:api:internal_url}}` resolves to the internal cluster URL of the "api" service. This means:

- Agent doesn't need to know the URL upfront
- If the API service gets redeployed at a different address, the frontend's env var updates automatically
- Internal traffic stays on the private network (faster, no egress cost)

### 2.2 Deploy Preview for Pull Requests

When an agent makes changes and pushes to a branch, auto-deploy a preview:

```json
{
  "preview": {
    "url": "https://my-app-pr-42.ml.ink",
    "branch": "fix/login-bug",
    "status": "running",
    "auto_cleanup": true,
    "ttl": "72h"
  }
}
```

This is huge for agent workflows: agent creates fix → pushes to branch → gets preview URL → can verify the fix works → creates PR.

### 2.3 Exec / Run One-Off Commands

Agents need to run database migrations, seed data, or debug:

```
exec_command(
  service="my-app",
  command="npx prisma migrate deploy"
)
# → Returns stdout/stderr

exec_command(
  service="my-app",
  command="node scripts/seed.js"
)
```

This is essential for agents that set up applications end-to-end. Without it, the agent deploys the app but can't run migrations — the app crashes, and the agent can't fix it without manual intervention.

### 2.4 Rollback

Agents make mistakes. They need to undo quickly:

```
rollback_service(name="my-app")
# → Rolls back to the previous working deployment
# → Returns the deployment that was restored

rollback_service(name="my-app", deployment_id="dep_abc")
# → Roll back to a specific deployment
```

The response should include:

```json
{
  "rolled_back_from": "dep_xyz",
  "rolled_back_to": "dep_abc",
  "status": "running",
  "url": "https://my-app.ml.ink",
  "reason": "Previous deployment (dep_xyz) was crash-looping"
}
```

### 2.5 Resource Templates / Stacks

Common architectures as one-call deploys:

```
create_stack(
  template="nextjs-sqlite",
  name="my-saas",
  repo="my-saas"
)
# → Creates service + SQLite database + correct env var wiring
#    + healthcheck configured + volume for /data if needed
```

Templates encode best practices that agents would otherwise have to discover by trial and error (correct port, correct build command, correct env var names for the framework).

### 2.6 Metrics and Resource Usage

Agents need to diagnose performance issues:

```
get_service(name="my-app", include_metrics=true)
```

```json
{
  "metrics": {
    "cpu_usage_percent": 85,
    "memory_usage_mb": 450,
    "memory_limit_mb": 512,
    "volume_usage_mb": 2048,
    "volume_limit_mb": 10240,
    "request_count_24h": 15420,
    "avg_response_time_ms": 230,
    "error_rate_percent": 2.3
  },
  "warnings": [
    "Memory usage is at 88% of limit. Consider increasing memory to 1024MB.",
    "CPU usage is high. Consider upgrading CPU allocation."
  ]
}
```

### 2.7 Custom Domains

```
add_domain(service="my-app", domain="app.example.com")
# → Returns: CNAME target, SSL status, verification instructions

get_domain_status(service="my-app", domain="app.example.com")
# → Returns: dns_verified, ssl_issued, propagation_status
```

---

## Tier 3: Ship Later

Features that build a moat and make your platform the default choice for every agent.

### 3.1 ~~Direct Code Deploy (No Git Required)~~ — Not Feasible Over MCP

**Why this doesn't work:** MCP is a pure HTTP/OAuth protocol where tool arguments pass through the LLM's context window. A base64-encoded tarball of even a small app would be megabytes — millions of tokens. There's no mechanism for streaming binary data or file uploads outside the tool call schema.

**The git flow is the right design.** `create_service` auto-creates the repo and returns `clone_url` + `git_token`. The agent pushes via git (which it already has access to in tools like Claude Code, Cursor, Windsurf, etc.). The webhook triggers the build. This is 1 MCP call + 1 git push — fast enough.

**If we ever wanted to skip git**, it would require a downloadable CLI or SDK that can upload files directly to our API outside the MCP protocol. That's a fundamentally different product surface and isn't worth building until there's clear demand.

### 3.2 Scheduled Tasks / Cron

```
create_cron(
  name="daily-cleanup",
  service="my-app",
  schedule="0 2 * * *",
  command="node scripts/cleanup.js"
)
```

Agents building SaaS apps almost always need scheduled tasks. Without this, they have to tell the human "you need to set up a cron job manually."

### 3.3 Multi-Region Deploy

```
create_service(
  name="my-app",
  repo="my-app",
  regions=["eu-west", "us-east"],
  routing="latency"            # Route users to nearest region
)
```

**Volume and Replica Constraints:**

Volumes are local block storage tied to a single cluster. They cannot be synced across regions or safely shared between replicas. These constraints must be enforced at the API level:

| Config                                   | Allowed | Why                                                          |
| ---------------------------------------- | ------- | ------------------------------------------------------------ |
| Multi-region + stateless (no volume)     | ✅ Yes  | Identical containers, latency-based routing                  |
| Multi-region + database resource (Turso) | ✅ Yes  | Turso natively handles multi-region replication              |
| Multi-region + volume                    | ❌ No   | Independent disks with no sync — data diverges immediately   |
| Multi-replica + stateless (no volume)    | ✅ Yes  | Standard horizontal scaling                                  |
| Multi-replica + volume                   | ❌ No   | ReadWriteOnce — only one pod can mount a block device safely |
| Single region + single replica + volume  | ✅ Yes  | The only safe volume config                                  |

**Error responses for invalid combos:**

```json
// Multi-region + volume
{
  "error": "multi_region_volume_conflict",
  "message": "Services with volumes cannot be deployed to multiple regions. Volumes are local storage tied to a single cluster with no cross-region sync.",
  "suggestion": "Deploy to one region, or remove the volume and use create_resource(type='sqlite') which supports multi-region natively via Turso."
}

// Multi-replica + volume
{
  "error": "replicas_volume_conflict",
  "message": "Services with volumes are limited to 1 replica. Multiple replicas cannot safely write to the same block device.",
  "suggestion": "Use replicas=1 with a volume, or remove the volume and use a database resource for shared state."
}
```

**Deployment note:** A service with a volume deployed to a single region causes brief downtime during redeploys (pod must unmount volume, new pod mounts it). This is the same tradeoff Railway makes. The `get_service` response should include `"volume_redeploy_downtime": true` so agents can warn users.

### 3.4 Secrets Management

Secrets are just env vars — there's no separate secrets API. They're set via `create_service(env_vars={...})` or `update_service(env_vars={...})` and returned in full via `get_service(include_env=true)`.

```json
{
  "env_vars": {
    "NODE_ENV": "production",
    "DATABASE_URL": "libsql://main-db-user.turso.io",
    "DATABASE_AUTH_TOKEN": "eyJhbGciOiJFZERTQSIs...",
    "STRIPE_KEY": "sk_live_abc123"
  }
}
```

**Why return full values:** Agents are stateless. A new conversation debugging a "401 Unauthorized" error needs to see the actual API key value to verify it's correct. The agent that set the secret may not be the same agent session that reads it back. The security boundary is the API key itself — anyone with a valid key owns the account.

**Storage:** Env vars are encrypted at rest in the database and injected into the container at runtime. They're never logged in build output or exposed in Kubernetes manifests that other tenants could access.

### 3.5 Collaborative Agent Features

When multiple agents (or multiple conversations) manage the same infrastructure:

```
list_services()
# → Returns ALL services for this user, with status, URL, last deploy time
# → An agent picking up where another left off can immediately understand the state
```

```json
{
  "services": [
    {
      "name": "api",
      "url": "https://api.ml.ink",
      "status": "running",
      "last_deployed": "2025-02-12T09:00:00Z",
      "repo": "my-org/api",
      "resources": [{ "name": "main-db", "type": "sqlite" }],
      "health": "healthy"
    },
    {
      "name": "frontend",
      "url": "https://frontend.ml.ink",
      "status": "crash_loop",
      "last_deployed": "2025-02-12T10:30:00Z",
      "repo": "my-org/frontend",
      "health": "unhealthy",
      "health_message": "Cannot connect to API_URL"
    }
  ]
}
```

A new agent conversation sees this and immediately knows: "frontend is broken because it can't reach the API — let me investigate."

### 3.6 Event Webhooks to the User

Let users receive webhooks when things happen:

```
create_webhook(
  url="https://my-slack-bot.com/deploy-notifications",
  events=["deploy.success", "deploy.failed", "service.unhealthy"]
)
```

This enables agent-built monitoring: agent deploys app + sets up webhook to Slack for deploy notifications.

### 3.7 Cost Estimation

Before deploying, agents should know the cost:

```
estimate_cost(
  services=[
    {name: "api", cpu: "1", memory: "512Mi"},
    {name: "frontend", cpu: "0.5", memory: "256Mi"}
  ],
  resources=[{type: "sqlite", size: "5Gi"}],
  volumes=[{size: "10Gi"}]
)
```

```json
{
  "estimated_monthly_cost_usd": 18.5,
  "breakdown": {
    "compute": 12.0,
    "storage_volumes": 2.5,
    "database": 4.0
  }
}
```

---

## Tool Design Guidelines

### Keep Tool Descriptions Lean

Every tool description is sent to the LLM on every request. Token bloat from verbose descriptions degrades agent performance. Write descriptions that are precise and scannable.

```
// BAD: 200 tokens per tool description
"Deploy a service from a git repository. This tool will clone the repository,
 detect the build pack, build the application using BuildKit, push the image
 to our internal registry, create Kubernetes resources including a Deployment,
 Service, and Ingress, configure TLS, set up auto-deploy webhooks, and return
 the public URL. Supports Node.js, Python, Go, Ruby, Rust, and static sites..."

// GOOD: 40 tokens
"Deploy a service from a git repo. Auto-detects language, builds, deploys,
 and returns the public URL. Supports auto-deploy on git push."
```

### Use Flat Parameter Names

Agents work better with flat parameters than deeply nested objects. Nested objects increase the chance of schema errors.

```
// BAD: Nested
{
  "service": {
    "config": {
      "resources": {
        "memory": "512Mi"
      }
    }
  }
}

// GOOD: Flat with clear prefixes
{
  "name": "my-app",
  "memory": "512Mi",
  "cpu": "1"
}
```

### Return Only What's Needed

Don't dump the entire service object when the agent only asked for logs. But DO include enough context that the agent doesn't need a follow-up call.

The sweet spot: return the fields relevant to the action taken + status + URL + any warnings.

### Use Enums, Not Free Text

Agents parse enums reliably. Free text requires interpretation.

```
// BAD
"status": "The service is currently experiencing issues and restarting"

// GOOD
"status": "crash_loop",
"health_message": "Container exited 3 times in 5 minutes. Last error: Cannot find module 'express'."
```

Standard status enums: `deploying`, `building`, `running`, `crashed`, `crash_loop`, `stopped`, `deleting`.

---

## Error Design

### Error Response Structure

Every error should follow this format:

```json
{
  "error": {
    "code": "service_not_found",
    "message": "No service named 'my-ap' found.",
    "suggestion": "Did you mean 'my-app'? Use list_services() to see all services.",
    "available_services": ["my-app", "my-api", "my-frontend"]
  }
}
```

### Common Error Patterns

| Error code                     | When                   | Suggestion to agent                                                                         |
| ------------------------------ | ---------------------- | ------------------------------------------------------------------------------------------- |
| `service_not_found`            | Wrong name             | Include list of similar names + suggest list_services()                                     |
| `build_failed`                 | Code doesn't compile   | Include last 20 lines of build log inline                                                   |
| `port_not_detected`            | Auto-detect failed     | "Specify port explicitly: port=3000"                                                        |
| `resource_limit`               | Over quota             | "User has 5/5 services. Delete one or upgrade plan."                                        |
| `deploy_in_progress`           | Already deploying      | "A deploy is in progress (started 30s ago). Use get_service() to check status."             |
| `repo_not_found`               | Wrong repo name/host   | "Repo 'my-app' not found on ml.ink. Use create_repo() first or specify host='github.com'."  |
| `crash_loop`                   | App keeps crashing     | Include last error line + suggest checking logs                                             |
| `volume_size_exceeded`         | Over storage limit     | "Volume 'data' is 10Gi limit. Current usage: 9.8Gi. Resize with resize_volume()."           |
| `multi_region_volume_conflict` | Volume + multi-region  | "Services with volumes cannot deploy to multiple regions. Use a database resource instead." |
| `replicas_volume_conflict`     | Volume + multi-replica | "Services with volumes are limited to 1 replica. Remove volume or use replicas=1."          |

### Build Failures: Inline the Error

When a build fails, the agent needs the build log immediately, not a pointer to it. Include the last 30 lines of build output directly in the error response:

```json
{
  "error": {
    "code": "build_failed",
    "message": "Build failed at step 'npm install'",
    "build_log_tail": [
      "npm ERR! 404 Not Found - GET https://registry.npmjs.org/expresss",
      "npm ERR! 404 'expresss@latest' is not in this registry.",
      "npm ERR! A complete log of this run can be found in:"
    ],
    "suggestion": "Check package.json for typos in dependency names. 'expresss' looks like a typo for 'express'."
  }
}
```

---

## The Full Tool Surface

Aim for 12-15 tools maximum. Every tool added consumes tokens on every agent request.

### Core (Ship Now)

| Tool               | Purpose                           | Key params                                                                                                                                              |
| ------------------ | --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `whoami`           | Auth check + account status       | —                                                                                                                                                       |
| `create_service`   | Deploy (with resources + volumes) | `repo, name, resources?, volumes?, env_vars?, port?, build_pack?, memory?, cpu?, replicas?, regions?, install_command?, build_command?, start_command?` |
| `get_service`      | Status + logs + metrics + env     | `name, deploy_log_lines?, runtime_log_lines?, include_env?, include_metrics?`                                                                           |
| `list_services`    | Overview of all services          | `status_filter?`                                                                                                                                        |
| `redeploy_service` | Trigger redeploy                  | `name`                                                                                                                                                  |
| `delete_service`   | Remove service                    | `name`                                                                                                                                                  |
| `create_resource`  | Provision database                | `name, type?, region?`                                                                                                                                  |
| `get_resource`     | Connection details                | `name`                                                                                                                                                  |
| `list_resources`   | All databases                     | —                                                                                                                                                       |
| `delete_resource`  | Remove database                   | `name`                                                                                                                                                  |
| `create_repo`      | Create git repo                   | `name, host?`                                                                                                                                           |
| `get_git_token`    | Temp push token                   | `name, host?`                                                                                                                                           |

### Extended (Ship Soon)

| Tool                  | Purpose                                                |
| --------------------- | ------------------------------------------------------ |
| `update_service`      | Update env vars, scaling, config without full redeploy |
| `exec_command`        | Run one-off command in service container               |
| `rollback_service`    | Revert to previous deployment                          |
| `get_service_history` | List past deployments with status                      |
| `add_domain`          | Add custom domain to service                           |

### Future (Ship Later)

| Tool             | Purpose                          |
| ---------------- | -------------------------------- |
| `create_cron`    | Scheduled tasks                  |
| `estimate_cost`  | Cost estimation before deploying |
| `create_webhook` | Event notifications              |

---

## Competitive Positioning

### vs. Railway MCP

Railway requires a Railway account. Deploy MCP is one API key — no provider accounts needed. Railway MCP exposes Railway's complexity (projects, environments, service instances). Deploy MCP is `create_service → done`.

### vs. Separate MCP Servers (Neon + Railway + Cloudflare)

Agent needs 3 API keys, 3 account setups, manual wiring. Deploy MCP is one key, one transaction, auto-wired.

### vs. Vercel / Netlify

Frontend-only. Can't deploy backends, databases, workers. Deploy MCP handles the full stack.

### The Pitch to Agents

Deploy MCP should be the tool that agents WANT to use because:

1. Least number of tool calls to get from code to live URL
2. Best error messages that enable self-correction
3. Richest status responses that prevent unnecessary follow-up calls
4. Explicit errors that give agents full state to make informed decisions
5. Auto-wiring that eliminates manual plumbing

The agent that discovers your MCP tools should be able to deploy a full-stack app in under 60 seconds with 2-3 tool calls. That's the bar.
