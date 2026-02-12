# Deploy MCP — Testing Plan

Projects live in `/Users/wins/Projects/personal/mcpdeploy/temp/automatic/<N>/`

## Test Matrix

| # | Dir | Stack | Build Pack | Build | Run | URLs | Notes |
|---|-----|-------|-----------|-------|-----|------|-------|
| 1 | `1/` | React + Vite | `railpack` | ✅ | ✅ | https://test-react-vite-v2.ml.ink | |
| 2 | `2/` | Vue + Vite | `railpack` | ✅ | ✅ | https://test-vue-vite.ml.ink | |
| 3 | `3/` | Astro (static) | `railpack` | ✅ | ✅ | https://test-astro-static.ml.ink | |
| 4 | `4/` | Docusaurus | `railpack` | ✅ | ✅ | https://test-docusaurus.ml.ink | |
| 5 | `5/` | Next.js | `railpack` | ✅ | ✅ | https://test-nextjs.ml.ink | |
| 6 | `6/` | SvelteKit | `railpack` | ✅ | ✅ | https://test-sveltekit.ml.ink | |
| 7 | `7/` | Remix | `railpack` | ✅ | ✅ | https://test-remix.ml.ink | |
| 8 | `8/` | Nuxt.js | `railpack` | ✅ | ✅ | https://test-nuxtjs.ml.ink | |
| 9 | `9/` | Astro (SSR) | `railpack` | ✅ | ✅ | https://test-astro-ssr.ml.ink | |
| 10 | `10/` | Express.js | `railpack` | ✅ | ✅ | https://test-express.ml.ink | |
| 11 | `11/` | Fastify | `railpack` | ✅ | ✅ | https://test-fastify.ml.ink | |
| 12 | `12/` | FastAPI | `railpack` | ✅ | ✅ | https://test-fastapi.ml.ink | |
| 13 | `13/` | Flask | `railpack` | ✅ | ✅ | https://test-flask.ml.ink | |
| 14 | `14/` | Django | `railpack` | ✅ | ✅ | https://test-django.ml.ink | |
| 15 | `15/` | Go (net/http) | `railpack` | ✅ | ✅ | https://test-go-api.ml.ink | |
| 16 | `16/` | Go (Gin) | `railpack` | ✅ | ✅ | https://test-go-gin.ml.ink | |
| 17 | `17/` | Bun + Hono | `railpack` | ✅ | ✅ | https://test-bun-hono.ml.ink | |
| 18 | `18/` | Ruby on Rails | `dockerfile` | ✅ | ✅ | https://test-rails.ml.ink | |
| 19 | `19/` | Spring Boot | `dockerfile` | ✅ | ✅ | https://test-spring-boot.ml.ink | |
| 20 | `20/` | Rust + Axum | `dockerfile` | ✅ | ✅ | https://test-rust-axum.ml.ink | |
| 21 | `21/` | Next.js full-stack | `railpack` | ✅ | ✅ | https://test-nextjs-fullstack.ml.ink | |
| 22 | `22/` | React + Express (mono) | `dockerfile` | ✅ | ✅ | https://test-mono-re.ml.ink | |
| 23 | `23/` | React + FastAPI (mono) | `dockerfile` | ✅ | ✅ | https://test-mono-rf.ml.ink | |
| 24 | `24/` | React + Go (mono) | `dockerfile` | ✅ | ✅ | https://test-mono-rg.ml.ink | |
| 25 | `25/` | Streamlit | `railpack` | ✅ | ✅ | https://test-streamlit.ml.ink | |
| 26 | `26/` | Gradio | `railpack` | ✅ | ✅ | https://test-gradio.ml.ink | |
| 27 | `27/` | WebSocket (Node) | `railpack` | ✅ | ✅ | https://test-websocket.ml.ink | |
| 28 | `28/` | T3 Stack | `railpack` | ✅ | ✅ | https://test-t3-stack.ml.ink | |
| 29 | `29/` | Flask (Dockerfile) | `dockerfile` | ✅ | ✅ | https://test-flask-dock.ml.ink | Port auto-detected from EXPOSE 5000 |
| 30 | `30/` | Plain HTML + assets | `static` | ✅ | ✅ | https://test-plain-html.ml.ink | |
| 31 | `31/` | 1 repo → 2 services (React + Express) | `dockerfile` | ⬜ | ⬜ | be: https://test-mono-31-be.ml.ink / fe: https://test-mono-31-fe.ml.ink | **Build-time env vars (dockerfile)**: fe passes `VITE_API_URL` pointing to be URL |
| 32 | `32/` | 1 repo → 2 services (Vue + FastAPI) | `railpack` | ⬜ | ⬜ | fe: https://test-mono-32-fe.ml.ink / be: https://test-mono-32-be.ml.ink | **Build-time env vars (railpack)**: fe passes `VITE_API_URL` pointing to be URL |
| 33 | `33/` | 1 repo → 2 services (API + Worker) | `dockerfile` | ✅ | ✅ | api: https://test-mono-33-api.ml.ink / wrk: https://test-mono-33-wrk.ml.ink | Retry: used wrong dockerfile_path on first attempt (`api.Dockerfile` vs `Dockerfile.api`) |
| 34 | `34/` | 1 repo → 2 services (React + Go) | `dockerfile` | ✅ | ✅ | api: https://test-mono-34-api.ml.ink / web: https://test-mono-34-web.ml.ink | Port auto-detected (api 3000, web 8080) |
| 35 | `35/` | 1 repo → 1 service (docs subdirectory) | `railpack` | ✅ | ✅ | https://test-mono-35-docs.ml.ink | |

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
| 31 | Frontend + Backend, each with own Dockerfile | `root_directory` + `dockerfile` build pack + **build-time env vars** (`VITE_API_URL`) |
| 32 | Frontend + Backend, no Dockerfiles at all | `root_directory` + `railpack` auto-detection + **build-time env vars** (`VITE_API_URL`) |
| 33 | API + Worker, multiple Dockerfiles at root | `dockerfile_path` (no `root_directory`) |
| 34 | Deeply nested services (`services/web/`, `services/api/`) | Deep `root_directory` paths |
| 35 | Docs site in a subdirectory | `root_directory` + `publish_directory` combo |

### Build-time env vars (#31, #32)

Vite/React/Vue bake `VITE_*` env vars into the JS bundle at build time. Tests #31 and #32 verify that env vars passed via `env_vars` in the MCP input reach the build step:

- **#31 (dockerfile)**: `VITE_API_URL=https://test-mono-31-be.ml.ink` passed as BuildKit `build-arg`. The React frontend Dockerfile must declare `ARG VITE_API_URL` and set `ENV VITE_API_URL=$VITE_API_URL` before `npm run build`.
- **#32 (railpack)**: `VITE_API_URL=https://test-mono-32-be.ml.ink` passed as a BuildKit secret. Railpack injects these automatically during build.

**Verification**: The frontend should display data fetched from the backend URL, confirming the env var was baked into the bundle (not hardcoded).
