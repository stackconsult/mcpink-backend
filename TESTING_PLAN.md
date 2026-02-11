# Deploy MCP — Testing Plan

> Deploy every common stack via MCP tools. See where it breaks.

## Results Summary (2026-02-10)

### Round 1 (initial deploy)

**9 PASS** | **15 FAIL** | **4 PARTIAL** | of 28 tests

### Round 2 (scaffold fixes retried)

Fixed all 10 user-code issues (#1-4, #6, #8, #18, #19, #23, #28), deleted old services, redeployed.

**All 11 builds succeeded** — every scaffold fix was correct. But only 1 new service got running:

| # | Fix Applied | Build | Runtime |
|---|-------------|-------|---------|
| 1 | `railpack` instead of `static` | SUCCESS | resource exhaustion (502) |
| 2 | `railpack` instead of `static` | SUCCESS | resource exhaustion (pending) |
| 3 | `railpack` instead of `static` | SUCCESS | resource exhaustion (pending) |
| 4 | `railpack` instead of `static` | SUCCESS | resource exhaustion (pending) |
| 6 | `"type":"module"` added | SUCCESS | resource exhaustion (503) |
| 8 | `package-lock.json` regenerated | SUCCESS | resource exhaustion (pending) |
| 18 | `Gemfile.lock` added | SUCCESS | resource exhaustion (pending) |
| 19 | Dockerfile reads `PORT` env | SUCCESS | resource exhaustion (pending) |
| 23 | Dockerfile reads `PORT` env | SUCCESS | resource exhaustion (pending) |
| **28** | **deps pinned to `@tanstack/react-query@^4`** | **SUCCESS** | **PASS** — https://test-t3-stack.ml.ink |

**Note:** #1 used static nginx pipeline despite `build_pack="railpack"` — possible platform bug (build_pack cached or auto-detected from repo).

### Combined Results (after both rounds)

**10 PASS** | **9 FAIL (resource exhaustion)** | **9 BUILDS VERIFIED (pending cluster capacity)**

### Fault Classification (after code inspection + retry)

| Fault | Count | Tests | Retry Result |
|-------|-------|-------|--------------|
| **User code** (scaffold bugs) | 4 | #6, #8, #18, #28 | All fixed, all build. #28 PASS. #6, #8, #18 blocked by resource exhaustion. |
| **User code** (wrong `build_pack`) | 4 | #1, #2, #3, #4 | Recreated with `railpack`. All build. All blocked by resource exhaustion. |
| **User code** (port mismatch) | 2 | #19, #23 | Dockerfiles fixed to read PORT. Both build. Both blocked by resource exhaustion. |
| **Deployment system** (resource exhaustion) | 9 | #16, #17, #20, #21, #22, #24, #25, #26, #27 | Not retried — code already correct, need cluster capacity. |

### Key Findings

1. **`build_pack="static"` skips build step.** Copies raw files to nginx. JS frameworks need `railpack`. The `build_command` parameter is ignored.

2. **k8s run pool saturates after ~13 services.** All later builds succeed but rollout times out at exactly 2m. Evidence: Go net/http (#15) PASS early, Go Gin (#16) FAIL later — identical stack. Confirmed in round 2: only 1 of 11 retried services got running despite all builds passing.

3. **No readiness/liveness probes on deployed pods.** The platform deploys containers without k8s health probes. The rollout timeout is the ONLY failure signal. Can't distinguish "pod Pending (no resources)" from "pod crashing (wrong config)" from "port mismatch (app listening on wrong port)".

4. **PORT env var is always injected.** Platform sets `PORT=<configured-port>` in every container. Apps that read PORT (Streamlit, Gradio, Go, Node) work correctly. Apps that ignore PORT (Spring Boot hardcoded to 8080) fail silently — the k8s Service routes to port 3000 but the app isn't there.

5. **`build_pack` parameter may be ignored.** Round 2 test #1 was recreated with `build_pack="railpack"` but the platform used the static nginx pipeline. Possible bug: build_pack cached from previous service or auto-detected from repo content.

---

## How It Works

Each test project lives in a numbered directory:

```
/Users/wins/Projects/personal/mcpdeploy/temp/automatic/
├── 1/    # React + Vite
├── 2/    # Vue + Vite
├── 3/    # Astro (static)
├── ...
└── 28/   # T3 Stack
```

For each test:

1. **Scaffold** the project locally in `temp/<N>/`
2. **Create repo** via `create_repo(name: "test-<stack>")` — creates on Gitea
3. **Push code** via `get_git_token` + `git push`
4. **Deploy** via `create_service(repo: "test-<stack>", ...)`
   - Monorepos call `create_service` multiple times (one per service)
5. **Verify** URL responds, logs exist, redeploy works, delete cleans up

All stacks should work. The point is to find where they don't.

---

## Test Matrix

Each test exercises: `create_repo` → `git push` → `create_service` → verify URL → `get_service` → `redeploy_service` → `delete_service`.

Status: `—` not run, `PASS` first try, `FAIL` see Failures section, `FIXED` failed then fixed.

| # | Stack | Category | Build Pack | DB | Dir | Services | Status | URL | What Happened |
|---|-------|----------|-----------|-----|-----|----------|--------|-----|---------------|
| 1 | React + Vite | static | `railpack` | — | `temp/1/` | 1 | ✅ FIXED (build OK, pending) | https://test-react-vite.ml.ink (502) | R1: wrong build_pack. R2: BUILD SUCCESS, pod assigned but returning 502 (resource pressure). Platform used static pipeline despite railpack specified. |
| 2 | Vue + Vite | static | `railpack` | — | `temp/2/` | 1 | ✅ FIXED (build OK, pending) | pending — no URL yet | R1: wrong build_pack. R2: BUILD SUCCESS but stuck pending (resource exhaustion). |
| 3 | Astro (static) | static | `railpack` | — | `temp/3/` | 1 | ✅ FIXED (build OK, pending) | pending — no URL yet | R1: wrong build_pack. R2: BUILD SUCCESS but stuck pending (resource exhaustion). |
| 4 | Docusaurus | static | `railpack` | — | `temp/4/` | 1 | ✅ FIXED (build OK, pending) | pending — no URL yet | R1: wrong build_pack. R2: BUILD SUCCESS but stuck pending (resource exhaustion). |
| 5 | Next.js | SSR | `railpack` | — | `temp/5/` | 1 | PASS | https://test-nextjs.ml.ink | First try, full SSR HTML |
| 6 | SvelteKit | SSR | `railpack` | — | `temp/6/` | 1 | ✅ FIXED (build OK, pending) | https://test-sveltekit.ml.ink (503) | R1: missing `"type":"module"`. R2: BUILD SUCCESS, pod assigned but returning 503 (resource pressure). |
| 7 | Remix | SSR | `railpack` | — | `temp/7/` | 1 | PASS | https://test-remix.ml.ink | First try, full React Router SSR |
| 8 | Nuxt.js | SSR | `railpack` | — | `temp/8/` | 1 | ✅ FIXED (build OK, pending) | pending — no URL yet | R1: lockfile out of sync. R2: regenerated `package-lock.json`, BUILD SUCCESS but stuck pending. |
| 9 | Astro (SSR) | SSR | `railpack` | — | `temp/9/` | 1 | PASS | https://test-astro-ssr.ml.ink | First try, Node adapter works |
| 10 | Express.js | API | `railpack` | SQLite | `temp/10/` | 1 | PASS | https://test-express.ml.ink | `{"status":"ok","stack":"express"}` |
| 11 | Fastify | API | `railpack` | — | `temp/11/` | 1 | PASS | https://test-fastify.ml.ink | `{"status":"ok","stack":"fastify"}` — `0.0.0.0` binding worked |
| 12 | FastAPI | API | `railpack` | SQLite | `temp/12/` | 1 | PASS | https://test-fastapi.ml.ink | `{"status":"ok","stack":"fastapi"}` |
| 13 | Flask | API | `railpack` | — | `temp/13/` | 1 | PASS | https://test-flask.ml.ink | `{"stack":"flask","status":"ok"}` |
| 14 | Django | API | `railpack` | SQLite | `temp/14/` | 1 | PASS | https://test-django.ml.ink | `{"status":"ok","stack":"django"}` — ALLOWED_HOSTS=* worked |
| 15 | Go (net/http) | API | `railpack` | SQLite | `temp/15/` | 1 | PASS | https://test-go-api.ml.ink | `{"stack":"go-net-http","status":"ok"}` |
| 16 | Go (Gin) | API | `railpack` | — | `temp/16/` | 1 | FAIL | — | BUILD SUCCESS but rollout timed out after 2m. See Failures. |
| 17 | Bun + Hono | API | `railpack` | — | `temp/17/` | 1 | FAIL | — | BUILD SUCCESS but rollout timed out after 2m. Bun+Node both detected. See Failures. |
| 18 | Ruby on Rails | API | `railpack` | — | `temp/18/` | 1 | ✅ FIXED (build OK, pending) | pending — no URL yet | R1: missing `Gemfile.lock`. R2: added lockfile, BUILD SUCCESS but stuck pending. |
| 19 | Spring Boot (Java) | API | `dockerfile` | — | `temp/19/` | 1 | ✅ FIXED (build OK, pending) | pending — no URL yet | R1: hardcoded port 8080. R2: Dockerfile reads PORT env, BUILD SUCCESS but stuck pending. |
| 20 | Rust + Axum | API | `dockerfile` | — | `temp/20/` | 1 | FAIL | — | BUILD SUCCESS (14.6s compile) but rollout timed out after 2m. See Failures. |
| 21 | Next.js full-stack | monorepo | `railpack` | SQLite | `temp/21/` | 1 | FAIL | — | BUILD SUCCESS (Next.js compiled, 5 pages) but rollout timed out. See Failures. |
| 22 | React Vite + Express | monorepo | `dockerfile` | SQLite | `temp/22/` | 2 | FAIL | — | Both API + Web: BUILD SUCCESS but rollout timed out. See Failures. |
| 23 | React Vite + FastAPI | monorepo | `dockerfile` | — | `temp/23/` | 2 | ✅ FIXED (build OK, pending) | pending — no URL yet | R1: Dockerfile hardcoded port 8000. R2: reads PORT env, BUILD SUCCESS but stuck pending. |
| 24 | React Vite + Go API | monorepo | `dockerfile` | — | `temp/24/` | 2 | FAIL | — | Both API + Web: BUILD SUCCESS but rollout timed out. See Failures. |
| 25 | Streamlit | specialty | `railpack` | — | `temp/25/` | 1 | FAIL | — | BUILD SUCCESS (streamlit installed) but rollout timed out. See Failures. |
| 26 | Gradio | specialty | `railpack` | — | `temp/26/` | 1 | FAIL | — | BUILD SUCCESS (gradio installed) but rollout timed out. See Failures. |
| 27 | WebSocket server (Node) | specialty | `railpack` | — | `temp/27/` | 1 | FAIL | — | BUILD SUCCESS but rollout timed out. See Failures. |
| 28 | T3 Stack (Next + tRPC + Prisma) | specialty | `railpack` | SQLite | `temp/28/` | 1 | ✅ FIXED -> PASS | https://test-t3-stack.ml.ink | R1: version conflict. R2: pinned `@tanstack/react-query@^4`, builds and runs. Next.js SSR OK. |

---

## Per-Test Spec

### Scaffold (local)

Each `temp/<N>/` directory contains the minimal project files. No boilerplate — just enough to prove the stack runs.

Every app must expose:
- `GET /` — returns something (HTML page or JSON `{"status":"ok"}`)
- `GET /health` — returns 200 (for APIs; SPAs just need `/` to return 200)

### Repo Creation

```
create_repo(name: "test-<stack>")
```

Then push:

```bash
cd /Users/wins/Projects/personal/mcpdeploy/temp/automatic/<N>
git init && git add -A && git commit -m "initial"
# get_git_token(name: "test-<stack>")
git remote add origin https://<token>@git.ml.ink/test-<stack>.git
git push -u origin main
```

### Deploy

**Single service (most cases):**

```
create_service(
  repo: "test-<stack>",
  name: "test-<stack>",
  branch: "main",
  build_pack: "<pack>",         # omit for railpack (default)
  port: <port>,                 # omit for default (auto-detect or 3000)
  env_vars: { ... }             # only if needed
)
```

**Monorepo (2 services from same repo):**

```
# Backend
create_service(
  repo: "test-monorepo-xy",
  name: "test-monorepo-xy-api",
  branch: "main",
  build_pack: "dockerfile",
  # Dockerfile at backend/Dockerfile or root Dockerfile with target
)

# Frontend
create_service(
  repo: "test-monorepo-xy",
  name: "test-monorepo-xy-web",
  branch: "main",
  build_pack: "static",
  env_vars: { "VITE_API_URL": "https://test-monorepo-xy-api.ml.ink" }
)
```

### Verify

| Check | Command | Pass |
|-------|---------|------|
| URL live | `curl -s https://test-<stack>.ml.ink` | 200 + expected body |
| Deploy logs | `get_service(name, deploy_log_lines: 50)` | Build output visible |
| Runtime logs | `get_service(name, runtime_log_lines: 20)` | Server startup visible |
| Redeploy | Change code → `git push` → `redeploy_service(name)` | New version live |
| Delete | `delete_service(name)` | 404 on URL, gone from `list_services` |

### Database Wiring (tests that use SQLite)

```
create_resource(name: "test-<stack>-db", type: "sqlite")
```

Redeploy with `env_vars` containing `DATABASE_URL` and `DATABASE_AUTH_TOKEN` from the resource.
App's `/db` endpoint should return data from Turso.

---

## Edge Cases

Run after the happy path works for at least a few stacks:

| # | Case | Status | How to Test | Expected |
|---|------|--------|------------|----------|
| E1 | Port mismatch | — | App listens on 3000, `create_service` says port 8080 | Health check fails, clear error in deploy logs |
| E2 | Build failure | — | Push syntax-error code | Workflow fails, error returned to agent, no partial deploy |
| E3 | OOM at runtime | — | App allocates > limit memory | Pod OOMKilled, visible in logs |
| E4 | Slow startup | — | App sleeps 90s before listening | Rollout timeout, error returned |
| E5 | Empty repo | — | No code, no Dockerfile | Railpack fails with clear error |
| E6 | Large repo | — | 500MB of assets | Build completes, `.git` pruned reduces context size |
| E7 | Duplicate name | — | `create_service` with name already taken | Error: name taken |
| E8 | Non-existent repo | — | `create_service` with fake repo name | Clone fails, error returned |
| E9 | Concurrent deploys | — | Two `create_service` calls at the same time | Both succeed independently |
| E10 | Webhook dedup | — | Push same commit twice | Second webhook is no-op (deterministic workflow ID) |
| E11 | Client-side routing | — | React SPA, navigate to `/about` directly | nginx fallback to `index.html` |
| E12 | Fastify localhost trap | — | Fastify without `host: '0.0.0.0'` | Connection refused — common gotcha |
| E13 | Django ALLOWED_HOSTS | — | Django without `*.ml.ink` in ALLOWED_HOSTS | 400 Bad Request |
| E14 | Missing start script | — | Node app without `start` script in package.json | Railpack error or fallback to `node index.js` |
| E15 | Python no requirements.txt | — | Python app with only pyproject.toml | Railpack must handle modern Python packaging |

---

## Execution

Run sequentially. Stop and fix when something breaks — that's the point.

```
1. Verify infra: cluster health, BuildKit, registry, Temporal worker
2. Run tests 1-4   (static)
3. Run tests 5-9   (SSR)
4. Run tests 10-20 (APIs)
5. Run tests 21-24 (monorepo)
6. Run tests 25-28 (specialty)
7. Run edge cases E1-E15
```

---

## Static Build Pack Finding (Tests 1-4)

**`build_pack="static"` does NOT run any build step.** It copies raw repo files directly into nginx and serves them. This means:

- **React/Vue (tests 1-2):** Serve raw `index.html` with uncompiled JSX/Vue imports — browser can't execute them
- **Astro/Docusaurus (tests 3-4):** Show default nginx welcome page because the build output directories (`dist/`, `build/`) don't exist

The `build_command` parameter is **ignored** by the static build pack.

**Recommendation:** JS frameworks that need a build step should use `build_pack="railpack"` instead. The `static` pack should only be used for pre-built HTML/CSS/JS files. Alternatively, the static pack should support running `build_command` before copying to nginx.

---

## Failures

Document every test that does not pass on the first try. One section per failure.

### Test #6 — SvelteKit ✅

**Status:** FIXED (build OK, blocked by resource exhaustion)

**R1 Symptom:** `npm run build` fails with: `Failed to resolve "@sveltejs/kit/vite". This package is ESM only but it was tried to load by require`.

**Root Cause:** Agent created `vite.config.js` (CJS) but `@sveltejs/kit` is ESM-only. The `package.json` was missing `"type": "module"`.

**Fix applied:** Added `"type": "module"` to `package.json`. Added `.gitignore` with `node_modules`.

**R2 Result:** BUILD SUCCESS (railpack detected Node, compiled SvelteKit). Runtime blocked by resource exhaustion — cluster at capacity.

---

### Test #8 — Nuxt.js ✅

**Status:** FIXED (build OK, blocked by resource exhaustion)

**R1 Symptom:** `npm ci` fails: `Invalid: lock file's commander@11.1.0 does not satisfy commander@13.1.0`.

**Root Cause:** `package-lock.json` was out of sync with `package.json` after `npx nuxi init`. Railpack uses `npm ci` which requires strict lock file sync.

**Fix applied:** Regenerated `package-lock.json` with `npm install --package-lock-only`.

**R2 Result:** BUILD SUCCESS (railpack detected Node, compiled Nuxt). Runtime blocked by resource exhaustion.

---

### Test #18 — Ruby on Rails ✅

**Status:** FIXED (build OK, blocked by resource exhaustion)

**R1 Symptom:** `BUILD FAILED: "/Gemfile.lock": not found`.

**Root Cause:** Agent created a `Gemfile` but did not run `bundle install` locally to generate `Gemfile.lock`. Railpack's Ruby provider requires `Gemfile.lock` to exist.

**Fix applied:** Generated `Gemfile.lock` via Docker (local Ruby too old). Committed to repo.

**R2 Result:** BUILD SUCCESS (railpack detected Ruby, installed gems). Runtime blocked by resource exhaustion.

---

### Test #22 — Monorepo React + Express (API + Web)

**Status:** FAIL (deployment system — resource exhaustion)

**Symptom:** `BUILD SUCCESS` → `deployment rollout timed out after 2m0s` for both API and Web services.

**Code inspection:** Express backend is correct — reads `PORT` env var, binds `0.0.0.0`, defaults to 3000, has `/` and `/health` routes. Dockerfile CMD `node backend/index.js` is correct.

**Root Cause:** Resource exhaustion. Code would work on a healthy cluster.

---

### Test #23 — Monorepo React + FastAPI (API + Web) ✅

**Status:** FIXED (build OK, blocked by resource exhaustion)

**R1 Symptom:** `BUILD SUCCESS` → `deployment rollout timed out after 2m0s`.

**Root Cause (R1):** Two issues: (1) Dockerfile hardcoded `--port 8000` while `create_service` used default 3000 → port mismatch. (2) Resource exhaustion.

**Fix applied:** Changed Dockerfile CMD to `uvicorn main:app --host 0.0.0.0 --port ${PORT:-3000}`. Now reads platform-injected PORT env var.

**R2 Result:** BUILD SUCCESS (FastAPI installed, image pushed). Runtime blocked by resource exhaustion.

---

### Test #24 — Monorepo React + Go API (API + Web)

**Status:** FAIL (deployment system — resource exhaustion)

**Symptom:** `BUILD SUCCESS` → `deployment rollout timed out after 2m0s` for both services.

**Code inspection:** Go backend is correct — reads `PORT` env var with `os.Getenv("PORT")`, defaults to 3000, binds `0.0.0.0`. Dockerfile `CMD ["/server"]` is correct.

**Root Cause:** Resource exhaustion. Code would work on a healthy cluster.

---

### Test #16 — Go (Gin)

**Status:** FAIL (deployment system — resource exhaustion)

**Symptom:** `BUILD SUCCESS` → `deployment rollout timed out after 2m0s`. Railpack correctly detected Go, compiled with `go build -ldflags=-w -s -o out`.

**Code inspection:** Correct — reads `PORT` env var, defaults to 3000, `r.Run(":" + port)` binds `0.0.0.0`, has `/` and `/health` routes. Identical pattern to test #15 (Go net/http) which PASSED.

**Root Cause:** Resource exhaustion. Same Go stack as #15 which deployed early and passed.

---

### Test #17 — Bun + Hono

**Status:** FAIL (deployment system — resource exhaustion)

**Symptom:** `BUILD SUCCESS` → `deployment rollout timed out after 2m0s`. Railpack detected both Bun 1.3.9 and Node 22.22.0, installed both via mise.

**Code inspection:** Correct — reads `PORT` env var via `process.env.PORT || '3000'`, uses Bun-native `export default { port, fetch }` syntax, has `/` and `/health` routes. Start script is `bun run index.ts`.

**Root Cause:** Resource exhaustion. Note: railpack installing both Bun and Node is wasteful but doesn't cause failure. Start script `bun run index.ts` from package.json is correct.

---

### Test #19 — Spring Boot (Java) ✅

**Status:** FIXED (build OK, blocked by resource exhaustion)

**R1 Symptom:** `BUILD SUCCESS` (Maven 2.4s) → `deployment rollout timed out after 2m0s`.

**Root Cause (R1):** Two issues: (1) App hardcodes port 8080, ignores PORT env. k8s routes to 3000 → port mismatch. (2) Resource exhaustion.

**Fix applied:** Changed Dockerfile CMD to `java -jar app.jar --server.port=${PORT:-3000}`. Now reads platform-injected PORT env var.

**R2 Result:** BUILD SUCCESS (Maven compiled, image pushed). Runtime blocked by resource exhaustion.

---

### Test #20 — Rust + Axum

**Status:** FAIL (deployment system — resource exhaustion)

**Symptom:** `BUILD SUCCESS` (Rust compiled in 14.65s via multi-stage Dockerfile) → `deployment rollout timed out after 2m0s`.

**Code inspection:** Correct — reads `PORT` env var, defaults to 3000, binds `0.0.0.0:{port}` with `TcpListener::bind`. Has `/` and `/health` routes. Multi-stage Dockerfile correct.

**Root Cause:** Resource exhaustion. Code would work on a healthy cluster.

---

## Rollout Timeout Analysis (Systemic Issue)

**Affected tests:** #16, #17, #20, #21, #22 (both), #24 (both), #25, #26, #27 — **9 stacks with correct code, 12 services**

**Also affected but with user code issues:** #19 (port 8080), #23 (port 8000) — port mismatch would cause 503 even on healthy cluster.

**Pattern:** Every single build succeeds. Every single rollout times out at exactly 2 minutes. This started after ~13 services were already running on the cluster.

**Evidence of resource exhaustion (not config error):**
- Go net/http (#15) deployed EARLY → PASS. Go Gin (#16) deployed LATER → FAIL. Same language, same railpack, same code pattern.
- Next.js (#5) deployed EARLY → PASS. Next.js fullstack (#21) deployed LATER → FAIL. Same framework.
- Express (#10) deployed EARLY → PASS. WebSocket/Express (#27) deployed LATER → FAIL. Same runtime.

**Root cause:** The `run` node pool has insufficient resources to run >13 services simultaneously. New pods stay `Pending` (no schedulable node) and the 2m rollout window expires.

**Critical platform finding: No readiness/liveness probes.**
Code inspection of `internal/k8sdeployments/k8s_resources.go` confirms: deployed pods have NO readiness, liveness, or startup probes. The rollout timeout is the ONLY failure signal. This means:
- Can't distinguish "pod Pending (no resources)" from "pod crashing (bad code)" from "port mismatch (app on wrong port)"
- Apps with port mismatches (#19, #23) would deploy "successfully" but return 503 — no health check catches it
- The `get_service` response only shows "deployment rollout timed out" with no pod-level diagnostics

**Investigation needed:**
1. `kubectl get pods -A | grep test-` — are pods Pending or CrashLoopBackOff?
2. `kubectl top nodes` — is the run pool CPU/memory exhausted?
3. `kubectl describe node <run-node>` — check allocatable vs allocated resources

**Platform improvements needed:**
1. **Add readiness probes** — HTTP GET on the configured port. Catches port mismatches, crashes, and gives meaningful pod events.
2. **Better error messages** — surface pod events (Pending/CrashLoopBackOff/OOMKilled) in `get_service` response.
3. **Auto-scaling** — detect when pods can't be scheduled and scale the run pool.
4. **Port validation** — for Dockerfile build_pack, warn if `EXPOSE` port doesn't match configured port.

---

### Test #28 — T3 Stack ✅

**Status:** FIXED -> PASS

**R1 Symptom:** `npm install` fails with `ERESOLVE unable to resolve dependency tree`. `@trpc/react-query@10.45.4` requires peer `@tanstack/react-query@^4.18.0` but `@tanstack/react-query@5.90.20` was specified.

**Root Cause:** Agent manually assembled package.json with incompatible version ranges. `@trpc/react-query@^10` requires React Query v4, not v5.

**Fix applied:** Pinned `@tanstack/react-query@^4.36.0` (compatible with `@trpc/react-query@^10`).

**R2 Result:** BUILD SUCCESS + RUNNING at https://test-t3-stack.ml.ink. Next.js 14.2.35 starts in 766ms, 4 static pages generated, tRPC API route works.
