# Agent-First Deploy MCP — Feature Strategy (My Version)

This plan assumes agents will deploy **100×** more apps than humans and will be retry-happy, dashboard-blind, and frequently stateless across sessions.

---

## 0) Pushback + resolution: “Idempotency” without ambiguity

Two different problems get called “idempotency”:

1. **Create semantics:** what happens if the thing already exists?
2. **Retry safety:** what happens if the client didn’t receive the response and tries again?

### Recommended compromise

- **Create stays create-only** (no magical upsert):
  - `create_service(name="api")` when `api` exists → **409 conflict** + full existing state + next actions.
- **Retries are safe via `idempotency_key`**:
  - same `(tenant, idempotency_key)` + same request hash → return same result; do not start duplicates.
  - same key reused with different params → `idempotency_key_reuse_conflict`.

This keeps behavior unambiguous while preventing duplicate builds/workflows from network retries.

Deletes should be idempotent (desired end state already achieved).

---

## 1) The agent reality (design constraints)

Agents are:
- high-throughput (many deploys per session)
- retry-happy (timeouts happen; agents retry)
- dashboard-blind (tool responses are their universe)
- stateless across sessions (tomorrow’s agent may not know what today’s agent did)

So your contract must optimize for:

- **URL + Debuggable State**
- **Safe retries**
- **Backpressure + cleanup**

---

## 2) Core principles

### Principle 1: Deployments are tracked operations
Build+deploy is long-running. Treat it as a task with progress, logs, cancellation, and resumability.

### Principle 2: Explicit state beats magical behavior
Create-only means create-only. Update-only means update-only. Replace is explicit and flagged.

### Principle 3: Retry-safe by design
Use `idempotency_key` to dedupe identical replays without turning create into upsert.

### Principle 4: Errors are documentation
Every error must answer:
- what happened
- what to do next
- relevant state now
- log tail for context

### Principle 5: Observability is part of the API
Logs, health, recent events, and deploy metadata are not “extras.” They’re the agent’s eyeballs.

### Principle 6: Lifecycle governance by default
Agents will create garbage. Ship TTL, quotas, and GC early.

### Principle 7: Small tool surface, high power
Aim for ~12–15 tools. Each tool should be expressive and self-describing.

---

## 3) Operation semantics

### Create vs Update vs Replace

**Create-only**
- Create if not exists
- If exists: conflict with full existing state

**Update**
- Modify mutable fields (env, cpu/mem, replicas, commands, buildpack overrides)
- No destructive changes unless explicitly requested

**Replace/Recreate**
- Explicit mode: `mode="recreate"`
- If volumes involved: require `allow_data_loss=true` (or refuse)

### Deletes are idempotent
`delete_service(name)` returns success even if already deleted.

### Mutating calls accept `idempotency_key?`
Rules:
- same key + same request hash → same result
- same key + different hash → error
- keys expire (e.g., 24h) to keep the table small

---

## 4) Minimum winning workflow (deploy any app)

Reality: code must reach you via a practical transport. Git is the correct baseline.

The irreducible flow:

1. `create_service(...)` (optionally auto-create repo for `ml.ink` host, return push creds)
2. agent pushes via git
3. webhook triggers build+deploy automatically

Optimization: for Gitea (`host=ml.ink`), `create_service` should auto-create the repo and return `clone_url + git_token` inline.

---

## 5) Tier 1 — Ship Now (foundation)

### 5.1 Task-native deploys
Make `create_service` and `redeploy_service` long-running tasks:
- progress updates (build step → push image → rollout → ready)
- cancellable
- resumable/pollable

### 5.2 One-call service + resources with auto-wiring
Support:

```json
create_service({
  "name": "my-saas",
  "repo": "my-saas",
  "resources": [{"type": "sqlite", "name": "main"}],
  "env_vars": {"NODE_ENV": "production"}
})
```

Server:
- provisions resource if requested
- injects connection env vars
- returns URL + what got wired

### 5.3 Conflict responses return full state
On duplicate create, return:
- existing URL/status/spec/resources/volumes
- clear “next action” suggestions: update / redeploy / delete+recreate

### 5.4 Debug-first `get_service`
Must include:
- status enum
- health reason code + message
- deploy log tail
- runtime log tail
- last deploy metadata (SHA, time, duration)
- effective config (port/buildpack detection results)

### 5.5 Backpressure + quotas
Must-have controls:
- concurrent builds per tenant/project
- services/resources caps
- rate limits on mutating calls
- “deploy in progress” handling (avoid duplicate build storms)

### 5.6 Transport safety hygiene
If using HTTP transport:
- validate Origin (DNS rebinding defense)
- support streaming/progress/log notifications where possible

---

## 6) Tier 2 — Ship Soon (competitive edge)

### 6.1 `update_service`
The explicit mutation tool:
- env vars
- scaling and limits
- command overrides/buildpack overrides
- volume attachment in explicit modes (add-only vs replace)

### 6.2 Service linking (dependency wiring)
Allow env var templates like:
- `{{service:api:internal_url}}`
- `{{service:api:url}}`

### 6.3 Preview environments + TTL
- branch/PR preview URLs by default
- auto-cleanup (e.g., 72h)
- “promote preview → prod”

### 6.4 Exec one-off commands
Controlled “run migrations/seed” tool with:
- timeouts
- output truncation
- audit logs

### 6.5 Releases + rollback
Model:
- Service (stable) → points to Release (immutable)
Rollbacks become pointer swaps.

---

## 7) Tier 3 — Ship Later (moat)

### 7.1 Stacks/templates
One-call architecture templates:
- Next.js + SQLite + auth
- FastAPI + SQLite + worker
- etc.

### 7.2 Domains with status tracking
- add domain
- track DNS verify + cert issuance
- expose status tool

### 7.3 Scheduled jobs (cron)
Agents will ask constantly. Add a first-class cron tool.

### 7.4 Cost estimation + budgets
Agents can create 500 previews in a day. Give them:
- preflight cost estimate
- tenant-level budgets/spend caps

---

## 8) Tight tool surface (suggested)

### Core (~12)
- `whoami`
- `create_service`
- `update_service`
- `get_service`
- `list_services`
- `redeploy_service`
- `delete_service`
- `create_resource`
- `get_resource`
- `list_resources`
- `delete_resource`
- `get_git_token` (or fold into create_service for ml.ink)

### Soon
- `exec_command`
- `rollback_service`
- `list_releases` / `get_service_history`
- `add_domain` / `get_domain_status`
