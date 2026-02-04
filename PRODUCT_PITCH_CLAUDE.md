# ml.ink: AI-Native Infrastructure for Sovereign Organizations

> **Deploy any application with AI—on infrastructure you control.**

---

## The Challenge

Organizations face a paradox. AI can now write complete applications—backends, APIs, dashboards—in minutes. But deploying them? That still requires:

- Manual cloud console clicks
- Configuration sprawl across services
- Weeks of procurement for approved vendors
- Security reviews that delay every change
- Infrastructure knowledge that's increasingly scarce

**The result:** AI accelerates development 100x, but deployment remains a bottleneck.

Meanwhile, governments and enterprises have unique constraints:
- Data cannot leave approved regions or facilities
- Infrastructure must run on audited, controlled hardware
- Vendor lock-in is a strategic risk
- Cloud providers may face jurisdiction issues

---

## The Solution: AI That Deploys to Your Infrastructure

ml.ink is infrastructure that AI agents can operate autonomously—on hardware you own and control.

```
Agent: create_app(repo="internal-dashboard", name="hr-portal")
→ 60 seconds later: "https://hr-portal.internal.gov is live"
```

No cloud accounts. No vendor dashboards. No tickets.

### What This Enables

| Traditional Approach | With ml.ink |
|---------------------|-------------|
| Submit infra request → Wait 2 weeks → Manual setup → Testing → Launch | AI agent deploys in under 2 minutes |
| Each service needs separate credentials and configuration | Single API key, single interface |
| Developers need infrastructure expertise | AI handles infrastructure, developers focus on applications |
| Every deployment requires security review | Security is built-in: sandboxed, hardened, audited |

---

## Technical Architecture

### Three-Plane Design

```
┌─────────────────────────────────────────────────────────────────┐
│ CONTROL PLANE (Your Network)                                    │
│  • ml.ink API - MCP protocol for AI agents                      │
│  • Workflow orchestration (Temporal)                            │
│  • Audit logs, access control, billing                          │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ BUILD PLANE (Your Data Center)                                  │
│  • Nixpacks auto-detection (Node, Python, Go, Rust, Java...)    │
│  • Container builds from source                                 │
│  • Private registry (images never leave your network)           │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ RUN PLANE (Your Hardware)                                       │
│  • Bare metal servers, VMs, or approved cloud                   │
│  • gVisor kernel sandboxing (defense-in-depth)                  │
│  • Egress firewall, capability dropping, resource limits        │
│  • Traffic never routes through external services               │
└─────────────────────────────────────────────────────────────────┘
```

### Key Properties

| Property | Implementation |
|----------|---------------|
| **Air-Gap Ready** | All components run on-premise. No external dependencies required. |
| **Data Sovereignty** | Code, builds, and containers stay on your infrastructure. |
| **Auditable** | Every action logged. Temporal workflows provide full replay. |
| **Portable** | Not locked to any cloud. Move between bare metal, private cloud, or hybrid. |

---

## Security Model

### Defense in Depth

**1. Container Isolation (gVisor)**
- User workloads run under gVisor, a user-space kernel
- Even if an application is compromised, it cannot escape to the host
- Protects against container escape CVEs that affect standard runtimes

**2. Network Controls**
- Egress firewall blocks unauthorized outbound connections
- Cloud metadata endpoints blocked (credential theft prevention)
- SMTP, mining pools, C&C ports blocked by default

**3. Resource Constraints**
- Memory, CPU, and process limits prevent resource exhaustion
- Fork bomb protection (PID limits)
- No capabilities by default (`--cap-drop=ALL`)

**4. Least Privilege**
- Containers run read-only by default
- No privilege escalation (`--security-opt=no-new-privileges`)
- Ephemeral storage only (no persistent access to host filesystem)

### What This Protects Against

| Threat | Standard Containers | With ml.ink Hardening |
|--------|--------------------|-----------------------|
| Kernel exploits | Host compromise | Blocked by gVisor |
| Credential theft | Access to cloud metadata | Egress blocked |
| Crypto mining | CPU exhaustion | Detected + killed |
| Spam campaigns | IP reputation damage | SMTP ports blocked |
| Container escape | Root on host | Sandboxed |

---

## Deployment Model

### Self-Hosted (Recommended for Government)

ml.ink runs entirely on your infrastructure:

- **Control Plane:** Your servers (2-4 vCPU, 8GB RAM)
- **Build Server:** Your servers (8-16 vCPU, 32GB RAM)
- **Run Servers:** Bare metal or VMs (scale as needed)

**Requirements:**
- Linux servers (Ubuntu 22.04+ or similar)
- Docker
- Network connectivity between planes (can be private VLAN)
- Optional: S3-compatible storage for backups

### Hybrid Option

- Control plane and builds on your infrastructure
- Run plane can include approved cloud regions for burst capacity
- Or vice versa: SaaS control plane with on-premise execution

---

## Integration

### Model Context Protocol (MCP)

ml.ink implements MCP—the emerging standard for AI tool use. Any MCP-compatible AI assistant can deploy:

- Claude (Anthropic)
- Custom AI agents
- Internal tooling with MCP adapters

### Available Operations

```
Core:
  create_app       Deploy application from Git repository
  redeploy         Trigger rebuild and deploy
  get_app_details  Status, logs, configuration
  delete_app       Remove application

Databases:
  create_resource  Provision database (SQLite/Turso)
  get_resource_details  Connection strings, credentials

Identity:
  whoami           Current user, permissions, quotas
```

### Example: AI-Driven Deployment

```
User: "Create a portal for HR to track leave requests"

AI Agent Actions:
1. create_github_repo(name="hr-leave-portal")
2. [Writes application code to repository]
3. create_app(repo="hr-leave-portal", name="leave-portal")
4. create_resource(name="leave-db", type="sqlite")
5. [Returns deployed URL to user]

Result: Production application deployed in minutes, not weeks.
```

---

## Disaster Recovery

### Resilience by Design

- **Applications survive control plane outage:** Deployed apps run independently. Loss of ml.ink API only prevents new deployments.
- **Automated backups:** Database and configuration backed up daily (configurable).
- **Documented recovery:** Complete disaster recovery runbooks included.

### Recovery Time Objectives

| Scenario | Running Apps | New Deployments | Recovery Time |
|----------|--------------|-----------------|---------------|
| Control plane down | Unaffected | Blocked | 2-4 hours |
| Build server down | Unaffected | Blocked | 1-2 hours |
| Run server down | Affected server only | Other servers OK | 30-60 min |

---

## Use Cases

### Government

**Rapid Prototyping**
- Policy teams describe applications to AI assistants
- Working prototypes deployed same day
- Iterate without IT bottleneck

**Internal Tools**
- HR systems, procurement dashboards, citizen service portals
- Deployed to internal infrastructure with full audit trail

**Classified Environments**
- Air-gapped deployment possible
- All components run on approved hardware
- No data exfiltration risk

### Enterprise

**Developer Productivity**
- AI agents handle infrastructure
- Developers focus on business logic
- Shadow IT eliminated (easy to deploy the right way)

**Multi-Region Compliance**
- Deploy to specific regions for data residency
- Same interface, different infrastructure backends

**Acquisition Integration**
- Rapidly deploy acquired company's applications
- Standardize on single deployment platform

---

## Comparison

| Capability | Traditional PaaS | Cloud-Native (K8s) | ml.ink |
|------------|-----------------|-------------------|--------|
| AI-native deployment | No | No | Yes |
| Runs on bare metal | No | Complex | Yes |
| Air-gap capable | No | Difficult | Yes |
| Time to first deploy | Hours/Days | Weeks | Minutes |
| Infrastructure expertise needed | Medium | High | Low |
| Container sandboxing | Basic | Optional | Default |
| Vendor lock-in | High | Medium | None |

---

## The Bigger Picture

### AI-Native Infrastructure

ml.ink represents a new category: infrastructure designed for AI operations.

Today's infrastructure was built for humans clicking dashboards. AI agents need:

- **Programmatic interfaces** (not UIs)
- **Composable operations** (not workflows)
- **Immediate feedback** (not tickets)
- **Clear security boundaries** (not trust assumptions)

### Where This Leads

```
2024: AI writes code
2025: AI deploys code ← We are here
2026: AI operates infrastructure
2027: AI designs architecture
```

Organizations that adopt AI-native infrastructure now will have:
- Faster iteration cycles
- Lower operational costs
- Reduced dependency on scarce DevOps talent
- A foundation for fully autonomous IT operations

---

## Getting Started

### Proof of Concept

1. **Provide infrastructure** (3 servers minimum: control, build, run)
2. **We deploy ml.ink** (2-4 hours for basic setup)
3. **Connect your AI assistant** (MCP configuration)
4. **Deploy your first application** (agent-driven, under 5 minutes)

### Engagement Options

| Option | Description |
|--------|-------------|
| **Self-Service** | Documentation, community support |
| **Assisted** | Installation support, 30-day onboarding |
| **Enterprise** | Dedicated support, SLA, custom integration |

---

## FAQ

**Q: Can this run completely air-gapped?**
A: Yes. All components (control plane, build server, registry, runtime) run on your infrastructure. External network access is optional.

**Q: What languages/frameworks are supported?**
A: Node.js, Python, Go, Rust, Ruby, Java, PHP, .NET, and more. Nixpacks auto-detects most projects. Custom Dockerfiles supported.

**Q: How does this compare to Kubernetes?**
A: ml.ink is simpler—no K8s expertise required. It's optimized for application deployment, not cluster management. Under the hood, it's containers with sophisticated orchestration.

**Q: What about existing CI/CD pipelines?**
A: ml.ink can complement or replace existing pipelines. Git push triggers automatic redeploy. Or use the API from your CI system.

**Q: Is the code open source?**
A: Core deployment engine (Coolify) is open source. ml.ink MCP layer can be deployed on-premise with enterprise license.

**Q: What happens if Anthropic/OpenAI changes their AI?**
A: ml.ink uses MCP, an open protocol. Any MCP-compatible agent works. You're not locked to any AI provider.

---

## Contact

Ready to deploy AI-native infrastructure?

**Email:** [contact info]
**Demo:** [scheduling link]

---

*ml.ink: Infrastructure for the agentic era.*
