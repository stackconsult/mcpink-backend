# Deploy MCP — Testing Plan

Projects live in `/Users/wins/Projects/personal/mcpdeploy/temp/automatic/<N>/`

## Test Matrix

| # | Dir | Stack | Build Pack | Build | Run | URLs | Issue |
|---|-----|-------|-----------|-------|-----|------|-------|
| 1 | `1/` | React + Vite | `railpack` | ✅ | ✅ | https://test-react-vite-v2.ml.ink | Fixed: publish_directory=dist |
| 2 | `2/` | Vue + Vite | `railpack` | ✅ | ✅ | https://test-vue-vite.ml.ink | Fixed: publish_directory=dist |
| 3 | `3/` | Astro (static) | `railpack` | ✅ | ✅ | https://test-astro-static.ml.ink | Fixed: publish_directory=dist |
| 4 | `4/` | Docusaurus | `railpack` | ✅ | ✅ | https://test-docusaurus.ml.ink | Fixed: publish_directory=build |
| 5 | `5/` | Next.js | `railpack` | ✅ | ✅ | https://test-nextjs.ml.ink | |
| 6 | `6/` | SvelteKit | `railpack` | ✅ | ✅ | https://test-sveltekit.ml.ink | Fixed: added `"start": "node build"` to package.json |
| 7 | `7/` | Remix | `railpack` | ✅ | ✅ | https://test-remix.ml.ink | |
| 8 | `8/` | Nuxt.js | `railpack` | ✅ | ✅ | https://test-nuxtjs.ml.ink | Fixed: was resource exhaustion |
| 9 | `9/` | Astro (SSR) | `railpack` | ✅ | ✅ | https://test-astro-ssr.ml.ink | |
| 10 | `10/` | Express.js | `railpack` | ✅ | ✅ | https://test-express.ml.ink | |
| 11 | `11/` | Fastify | `railpack` | ✅ | ✅ | https://test-fastify.ml.ink | |
| 12 | `12/` | FastAPI | `railpack` | ✅ | ✅ | https://test-fastapi.ml.ink | |
| 13 | `13/` | Flask | `railpack` | ✅ | ✅ | https://test-flask.ml.ink | |
| 14 | `14/` | Django | `railpack` | ✅ | ✅ | https://test-django.ml.ink | |
| 15 | `15/` | Go (net/http) | `railpack` | ✅ | ✅ | https://test-go-api.ml.ink | |
| 16 | `16/` | Go (Gin) | `railpack` | ✅ | ✅ | https://test-go-gin.ml.ink | Fixed: was resource exhaustion |
| 17 | `17/` | Bun + Hono | `railpack` | ✅ | ✅ | https://test-bun-hono.ml.ink | Fixed: was resource exhaustion |
| 18 | `18/` | Ruby on Rails | `dockerfile` | ✅ | ✅ | https://test-rails.ml.ink | Fixed: switched to Dockerfile with explicit puma CMD (railpack detected wrong start command) |
| 19 | `19/` | Spring Boot | `dockerfile` | ✅ | ✅ | https://test-spring-boot.ml.ink | Fixed: was resource exhaustion |
| 20 | `20/` | Rust + Axum | `dockerfile` | ✅ | ✅ | https://test-rust-axum.ml.ink | Fixed: simplified Dockerfile (dependency caching trick caused stale build) |
| 21 | `21/` | Next.js full-stack | `railpack` | ✅ | ✅ | https://test-nextjs-fullstack.ml.ink | Fixed: was resource exhaustion |
| 22 | `22/` | React + Express (mono) | `dockerfile` | ✅ | ✅ | https://test-mono-re.ml.ink | Single service: backend serves API + React frontend |
| 23 | `23/` | React + FastAPI (mono) | `dockerfile` | ✅ | ✅ | https://test-mono-rf.ml.ink | Single service: backend serves API + React frontend |
| 24 | `24/` | React + Go (mono) | `dockerfile` | ✅ | ✅ | https://test-mono-rg.ml.ink | Single service: backend serves API + React frontend |
| 25 | `25/` | Streamlit | `railpack` | ✅ | ✅ | https://test-streamlit.ml.ink | Fixed: was resource exhaustion |
| 26 | `26/` | Gradio | `railpack` | ✅ | ✅ | https://test-gradio.ml.ink | Fixed: was resource exhaustion |
| 27 | `27/` | WebSocket (Node) | `railpack` | ✅ | ✅ | https://test-websocket.ml.ink | Fixed: was resource exhaustion |
| 28 | `28/` | T3 Stack | `railpack` | ✅ | ✅ | https://test-t3-stack.ml.ink | |
| 29 | `29/` | Flask (Dockerfile) | `dockerfile` | ✅ | ✅ | https://test-flask-dockerfile.ml.ink | Validates dockerfile build pack with Python |
| 30 | `30/` | Plain HTML + assets | `static` | ✅ | ✅ | https://test-plain-html.ml.ink | No build step — raw HTML/CSS/JS served via nginx |
| 31 | `31/` | 1 repo → 2 services (React + Express) | `dockerfile` | ✅ | ⚠️ | be: https://test-mono-31-be.ml.ink | be ✅/✅, fe ✅/❌: build succeeded but workflow never transitions to deploy |
| 32 | `32/` | 1 repo → 2 services (Vue + FastAPI) | `railpack` | ✅ | ✅ | fe: https://test-mono-32-fe.ml.ink / be: https://test-mono-32-be.ml.ink | Both services live — `root_directory` + railpack works |
| 33 | `33/` | 1 repo → 2 services (API + Worker) | `dockerfile` | ✅ | ✅ | api: https://test-mono-33-api.ml.ink / wrk: https://test-mono-33-wrk.ml.ink | Both services live — `dockerfile_path` works |
| 34 | `34/` | 1 repo → 2 services (React + Go) | `dockerfile` | ✅ | ⚠️ | api: https://test-mono-34-api.ml.ink | api ✅/✅, web ✅/❌: same bug as 31-fe |
| 35 | `35/` | 1 repo → 1 service (docs subdirectory) | `railpack` | ✅ | ✅ | https://test-mono-35-docs.ml.ink | `root_directory=docs` + `publish_directory=build` works |

**33 ✅** | **0 ❌** | **2 ⚠️** (workflow stuck after build — see below)

---

## Static Sites (require `publish_directory`)

These frameworks build to static files and need `publish_directory` set so the platform uses the railpack static build flow (build app, then serve output via nginx on port 8080):

| Stack | `publish_directory` |
|-------|-------------------|
| React + Vite | `dist` |
| Vue + Vite | `dist` |
| Astro (static) | `dist` |
| Docusaurus | `build` |

---

## Full-Stack / Monorepo Apps

### Single-image monorepos (#22-24)

Tests #22-24 use a single `dockerfile` service with a multi-stage Docker build:
1. **Stage 1**: Build React frontend with Vite (`npm run build` → `dist/`)
2. **Stage 2**: Build/install backend
3. **Runtime**: Backend serves both the API (`/api/items`) and the built React static files (`/`)

The React frontend fetches `/api/items` on load and renders the response — demonstrating real end-to-end connectivity (not two independent hello-world services).

### Multi-service monorepos (#31-35) — `build_config` tests

These test the `root_directory`, `dockerfile_path`, and `publish_directory` fields in `build_config` JSONB. Each repo deploys as **multiple independent services**.

| # | Pattern | `build_config` fields tested |
|---|---------|------------------------------|
| 31 | Frontend + Backend, each with own Dockerfile | `root_directory` + `dockerfile` build pack |
| 32 | Frontend + Backend, no Dockerfiles at all | `root_directory` + `railpack` auto-detection |
| 33 | API + Worker, multiple Dockerfiles at root | `dockerfile_path` (no `root_directory`) |
| 34 | Deeply nested services (`services/web/`, `services/api/`) | Deep `root_directory` paths |
| 35 | Docs site in a subdirectory | `root_directory` + `publish_directory` combo |

---

## Remaining Issues

### #31-fe, #34-web — Workflow stuck after successful build

Both are multi-stage Dockerfile builds (node → nginx) with `root_directory` set. Build logs show `BUILD SUCCESS` and image pushed to registry, but:
- `build_status` stays `building` forever
- `updated_at` never changes from creation time
- `runtime_status` stays `pending`

The deploy activity (K8s resource creation) never fires. Same repo's other service (simple single-stage Dockerfile + `root_directory`) deployed fine.

**Common pattern**: `build_pack=dockerfile` + `root_directory` + multi-stage Dockerfile with nginx. Simpler Dockerfiles with `root_directory` work (31-be Express, 34-api Go).

