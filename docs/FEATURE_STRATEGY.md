# Agent-First Deployment MCP: Feature Strategy

> Agents deploy 100x more apps than humans. Design every feature for the agent first, human second.

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

### 3. Idempotent Everything

Agents may call the same tool twice (retry on timeout, re-run after context loss, etc.). Every operation must be safe to repeat.

```
# Calling create_service("my-app") twice should NOT fail.
# Second call returns the existing service if config matches.
# Second call with different config returns a clear diff of what changed.
```

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
        "DATABASE_AUTH_TOKEN": "***injected***"
      }
    }
  ],
  "volumes": [{ "mount_path": "/data", "size": "10Gi", "replicas": 2 }],
  "auto_deploy": true,
  "webhook_active": true
}
```

### 1.2 Create Repo + Push in One Flow

Agents generate code locally. They need to get it deployed. The fastest path:

```
# Step 1: Create repo + get push token in one call
create_repo(name="my-app")
# → Returns: repo_url, git_token, clone_url

# Step 2: Agent pushes code using the token
# (agent runs: git remote add origin <clone_url> && git push)

# Step 3: Auto-deploy triggers via webhook
# OR agent explicitly calls create_service if webhook not yet set up
```

**Better flow for the future:** `deploy_code` tool that accepts a tarball/zip of code directly, skipping the git step entirely. This is the ultimate agent flow — no git knowledge required.

### 1.3 Rich Logs in get_service

Agents debug by reading logs. Make logs a first-class tool output, not an afterthought.

```
get_service(
  name="my-app",
  deploy_log_lines=100,     # Last N lines of build/deploy output
  runtime_log_lines=50,     # Last N lines of application stdout/stderr
  include_env=true           # Show current env vars (redacted secrets)
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
    "DATABASE_URL": "libsql://***",
    "DATABASE_AUTH_TOKEN": "***redacted***"
  }
}
```

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

### 1.5 Idempotent Operations

| Operation                    | Idempotency behavior                                                                                 |
| ---------------------------- | ---------------------------------------------------------------------------------------------------- |
| `create_service("my-app")`   | If exists with same config → return existing. If exists with different config → return diff + error. |
| `create_resource("my-db")`   | If exists → return existing connection info.                                                         |
| `create_repo("my-app")`      | If exists → return existing repo URL + token.                                                        |
| `delete_service("my-app")`   | If already deleted → return success (not 404).                                                       |
| `redeploy_service("my-app")` | Safe to call multiple times — deduped by commit SHA.                                                 |

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

### 3.1 Direct Code Deploy (No Git Required)

The ultimate agent flow — skip git entirely:

```
deploy_code(
  name="my-app",
  code=<base64_tarball>,       # Or a URL to a tarball
  runtime="node20",
  start_command="node server.js",
  env_vars={"PORT": "3000"}
)
```

Why this matters: Many agent workflows generate code in memory or in a temp directory. Requiring them to create a repo, get a token, git init, commit, push, then deploy is 6 steps. This makes it one step.

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

### 3.4 Secrets Management

```
set_secret(name="my-app", key="STRIPE_KEY", value="sk_live_...")
# → Encrypted at rest, injected as env var at runtime
# → Never returned in plain text in any tool response

list_secrets(name="my-app")
# → Returns key names only, never values
# → ["STRIPE_KEY", "SENDGRID_KEY"]
```

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

| Error code             | When                 | Suggestion to agent                                                                        |
| ---------------------- | -------------------- | ------------------------------------------------------------------------------------------ |
| `service_not_found`    | Wrong name           | Include list of similar names + suggest list_services()                                    |
| `build_failed`         | Code doesn't compile | Include last 20 lines of build log inline                                                  |
| `port_not_detected`    | Auto-detect failed   | "Specify port explicitly: port=3000"                                                       |
| `resource_limit`       | Over quota           | "User has 5/5 services. Delete one or upgrade plan."                                       |
| `deploy_in_progress`   | Already deploying    | "A deploy is in progress (started 30s ago). Use get_service() to check status."            |
| `repo_not_found`       | Wrong repo name/host | "Repo 'my-app' not found on ml.ink. Use create_repo() first or specify host='github.com'." |
| `crash_loop`           | App keeps crashing   | Include last error line + suggest checking logs                                            |
| `volume_size_exceeded` | Over storage limit   | "Volume 'data' is 10Gi limit. Current usage: 9.8Gi. Resize with resize_volume()."          |

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

| Tool               | Purpose                           | Key params                                                                                                                         |
| ------------------ | --------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `whoami`           | Auth check + account status       | —                                                                                                                                  |
| `create_service`   | Deploy (with resources + volumes) | `repo, name, resources?, volumes?, env_vars?, port?, build_pack?, memory?, cpu?, install_command?, build_command?, start_command?` |
| `get_service`      | Status + logs + metrics + env     | `name, deploy_log_lines?, runtime_log_lines?, include_env?, include_metrics?`                                                      |
| `list_services`    | Overview of all services          | `status_filter?`                                                                                                                   |
| `redeploy_service` | Trigger redeploy                  | `name`                                                                                                                             |
| `delete_service`   | Remove service                    | `name`                                                                                                                             |
| `create_resource`  | Provision database                | `name, type?, region?`                                                                                                             |
| `get_resource`     | Connection details                | `name`                                                                                                                             |
| `list_resources`   | All databases                     | —                                                                                                                                  |
| `delete_resource`  | Remove database                   | `name`                                                                                                                             |
| `create_repo`      | Create git repo                   | `name, host?`                                                                                                                      |
| `get_git_token`    | Temp push token                   | `name, host?`                                                                                                                      |

### Extended (Ship Soon)

| Tool                  | Purpose                                                |
| --------------------- | ------------------------------------------------------ |
| `update_service`      | Update env vars, scaling, config without full redeploy |
| `exec_command`        | Run one-off command in service container               |
| `rollback_service`    | Revert to previous deployment                          |
| `get_service_history` | List past deployments with status                      |
| `add_domain`          | Add custom domain to service                           |

### Future (Ship Later)

| Tool             | Purpose                                |
| ---------------- | -------------------------------------- |
| `deploy_code`    | Deploy from tarball/zip, no git needed |
| `create_cron`    | Scheduled tasks                        |
| `estimate_cost`  | Cost estimation before deploying       |
| `create_webhook` | Event notifications                    |

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
4. Idempotent operations that are safe to retry
5. Auto-wiring that eliminates manual plumbing

The agent that discovers your MCP tools should be able to deploy a full-stack app in under 60 seconds with 2-3 tool calls. That's the bar.
