# ml.ink: Sovereign Deployment for AI Agents
## A private “ship button” for governments and enterprises

### Executive summary

**ml.ink** is a deployment control plane designed for the agentic era: it lets AI coding agents (including **OpenAI Codex**, Claude, Cursor, Windsurf, and any MCP-capable agent) **provision and deploy applications into your own infrastructure**—private bare metal, private cloud, or on‑prem—through a single, auditable interface.

AI can already generate working software in minutes. The bottleneck is the “last mile”: credentials, environments, DNS/TLS, databases, resource limits, and safe runtime isolation. **ml.ink turns that last mile into an API** so your organization can go from “code written” to “service running” without handing agents the keys to the kingdom or pushing sensitive workloads into a public PaaS.

---

### The problem: AI can build; your org can’t safely ship

Enterprises and agencies are adopting AI assistants to accelerate delivery, but deployment remains:

- **Fragmented**: one tool for hosting, another for databases, another for DNS/TLS, secrets, logs, backups.
- **Manual**: human operators bridge connection strings, dashboards, permissions, ports, and environments.
- **Risky**: AI-generated code and dependencies are untrusted by default; production environments are not.
- **Sovereignty constrained**: many workloads can’t use convenient public PaaS offerings due to data residence, policy, procurement, or mission assurance requirements.

The result is predictable: agents can write software quickly, but **your throughput is still gated by DevOps and compliance**.

---

### The solution: an agent-native control plane that runs on your hardware

ml.ink exposes a stable, high-level contract (MCP tools like `create_app`, `list_apps`, `get_app_details`, `create_resource`) that agents can use to deploy real services:

- **One call** to deploy an app (build + registry + runtime + domain).
- **Repeatable workflows** orchestrated as durable jobs (retries, idempotency, observability).
- **Provider abstraction**: agents operate on “apps” and “resources,” not vendor dashboards.

Under the hood, ml.ink intentionally builds on boring, battle-tested primitives (Docker, SSH, standard registries) with a modern workflow engine and an app deployment backend (Coolify) to avoid lock‑in and to make audits and operations tractable.

---

### Deployment model: the 3‑plane architecture (safety + performance + clarity)

ml.ink separates “control,” “build,” and “run” concerns into independent planes so untrusted code can’t easily impact the systems that manage it.

**Plane A — Control Plane (you own)**
- The ml.ink API / MCP server (the “agent contract”)
- Metadata DB (projects, resources, usage, audit)
- Authentication (API keys) and integrations (Git providers)

**Plane B — Factory (build + orchestration backend)**
- Coolify master (UI + API) orchestrating deployments via SSH
- Build engine (Nixpacks / Dockerfile / Compose build packs)
- Registry access (shared image store when you have multiple servers)

**Plane C — Muscle (runtime)**
- Docker engine + per-host reverse proxy
- User workloads (apps, APIs, workers)
- Optional scale-to-zero for idle services (on-demand mode)

This separation is practical: builds are bursty and CPU/RAM heavy, while production runtimes require predictable latency and stability.

---

### What you can deploy

ml.ink is built to deploy the things enterprises and governments actually ship:

- **Web apps** (SSR or static)
- **APIs** (Go, Node, Python, etc.)
- **Backends and services** (workers, cron jobs, WebSocket services)
- **Multi-container stacks** (Compose)

Agents can also provision supporting **resources** (today: managed SQLite via Turso; roadmap: Postgres/Redis and bring‑your‑own connection strings).

---

### Enterprise & government-grade controls (without “trusting the agent”)

ml.ink assumes AI-generated code is untrusted and designs for containment:

**1) Runtime isolation with gVisor (runsc)**
- User workloads can run under **gVisor** for kernel-level isolation (container escape risk reduction).
- Practical compatibility is addressed via a tested configuration (hostinet mode) and deployment integration.

**2) Guardrails against common abuse**
- Host-level egress controls to block high-risk destinations and ports (e.g., metadata endpoints, SMTP).
- Support for strict container runtime settings (capability drops, PID limits, CPU/memory limits).
- Operational checks and verification scripts for new runtime servers.

**3) Private networking and reduced blast radius**
- Internal traffic can stay on private networks (e.g., Hetzner vSwitch / VLAN) for registry pulls and server-to-server control.
- Clear separation between trusted “ops” services (registry, Git, monitoring) and untrusted user workloads.

**4) Auditability**
- API-key-based agent authentication supports consistent attribution.
- The control plane can maintain an audit trail of actions (deploy intent, resources, usage).

**5) Resilience by design**
- Deployed apps keep running even if the build/orchestration backend is down because runtime servers run standard Docker services locally.
- Disaster recovery is runbook-driven with scripted backups and restoration procedures.

---

### Sovereignty and “private by default” options

ml.ink is designed to run in the environments that procurement and policy demand:

- **Private bare metal** (high compute density and cost efficiency)
- **Private cloud** (your AWS/Azure/GCP accounts, or other providers)
- **On‑prem data centers** and segmented networks
- **Internal Git** options for sensitive code (e.g., Gitea/Forgejo), including deploy-key based builds

For externally reachable applications, DNS/TLS can be managed in a way that fits your posture (including optional Cloudflare fronting for predefined domains to reduce origin exposure and add basic DDoS protection).

---

### Use cases (what this enables in practice)

**Secure internal tooling**
- Analysts and operators can request an agent-built dashboard or workflow tool and have it deployed to an internal subdomain with enforced guardrails.

**Isolated research and evaluation sandboxes**
- Rapidly stand up ephemeral environments for evaluating open-source tools, models, or codebases, while containing risk.

**Mission systems support**
- Deploy and update services on your own runtime fleet with predictable placement, resource tiers, monitoring, and runbooks.

**Modernization at the speed of intent**
- Replace ticket-driven “please deploy this” workflows with an audited, policy-bound interface that agents can call.

---

### Why ml.ink (the differentiators)

- **Agent-native interface**: MCP is the contract; agents don’t need a human to click dashboards.
- **Runs in your infrastructure**: data sovereignty and policy alignment without needing a public PaaS.
- **Safety via architecture**: separation of planes + sandboxed runtime + network controls.
- **Operational reality**: backups, monitoring, and disaster recovery are first-class, not afterthoughts.
- **Composable and replaceable**: underlying providers and components can evolve without breaking the agent contract.

---

### A pragmatic pilot path

A typical enterprise/government rollout can be staged:

1. **Pilot** (single Factory + single Muscle + optional Ops node)
2. **Harden** (gVisor, egress policy, resource tiers, audit requirements)
3. **Scale out** (multiple Muscle nodes, dedicated build servers, internal registry, internal Git)
4. **Govern** (policy templates for allowed service types, network egress, resource caps, and placement)

---

### Bottom line

AI is already changing software creation; the strategic bottleneck is **safe execution**. ml.ink gives governments and enterprises a way to let agents *ship*—without surrendering sovereignty, security posture, or operational control.

If you want a single sentence:

> **ml.ink turns your private infrastructure into an agent-addressable platform—so “build” and “deploy” become auditable API calls, not human bottlenecks.**

