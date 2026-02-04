# ml.ink: The Infrastructure Layer for Sovereign AI
## A Private "Internet for Agents" for Government & Enterprise

### Executive Summary

**ml.ink** is an infrastructure-agnostic platform designed to bridge the gap between Autonomous AI Agents and secure, production-grade deployment.

While modern AI agents (Claude, Cursor, Windsurf) can generate code in seconds, deploying that code securely remains a bottleneck requiring human intervention. **ml.ink** solves this by providing a standardized Machine Control Protocol (MCP) interface that allows AI to provision, deploy, and manage applications autonomously—**strictly within your private infrastructure.**

For governments and enterprises, this offers the agility of the public cloud with the **security, sovereignty, and cost-efficiency of private bare metal.**

---

### The Problem: The "Last Mile" Gap in AI Utility

Agencies and enterprises are adopting AI coding assistants to accelerate development. However, these agents hit a hard wall when it comes to execution:

1.  **Security Risks:** Public cloud deployments expose sensitive data to third-party providers.
2.  **Sovereignty Issues:** Data residence requirements often preclude using convenient PaaS solutions (like Vercel or Heroku).
3.  **Manual Friction:** An agent can write an internal tool in 30 seconds, but a human engineer spends 2 hours configuring DNS, SSL, databases, and firewalls.

**The Result:** Your AI workforce is capable of building, but paralyzed by an inability to ship.

---

### The Solution: Autonomous Deployment on Sovereign Metal

**ml.ink** transforms your raw infrastructure (dedicated servers, private clouds, on-prem data centers) into an agent-addressable platform.

*   **Agent-Native:** It speaks the language of AI (MCP). Agents simply call `create_app` or `create_database`.
*   **Infrastructure Agnostic:** Deploy to Hetzner, AWS, Azure, or your own air-gapped data centers.
*   **Zero-Trust by Design:** Untrusted code is isolated by default.

---

### Core Capabilities for Enterprise & Defense

#### 1. Total Data Sovereignty
Unlike public PaaS offerings, **ml.ink** installs on your hardware. You own the Control Plane (Plane A), the Build Factory (Plane B), and the Runtime Muscle (Plane C).
*   **Private Networking:** Internal traffic stays on vSwitches/VLANs.
*   **No Vendor Lock-in:** The underlying orchestration relies on standard Docker and SSH.
*   **Audit Trails:** Every agent action is authenticated, logged, and reversible.

#### 2. Defense-Grade Isolation (gVisor)
We assume AI-generated code—and the libraries it pulls—cannot be fully trusted. **ml.ink** implements a **3-Plane Architecture** with rigorous sandboxing:
*   **Control Plane Separation:** Builds happen on physically separate servers (Plane B) from runtimes (Plane C) to prevent resource exhaustion attacks.
*   **Kernel-Level Sandboxing:** User workloads run inside **gVisor** (Google's userspace kernel), preventing container escape attacks. Even if an application is compromised, the host kernel remains untouched.
*   **Egress Filtering:** Strict firewall rules block unauthorized outbound connections (e.g., to crypto mining pools, Tor nodes, or cloud metadata endpoints).

#### 3. Operational Resilience
Built on **Temporal** workflows, the platform enables "self-healing" infrastructure.
*   **Idempotency:** Deployments are deterministic.
*   **Disaster Recovery:** The "Muscle" (runtime) continues serving traffic even if the "Factory" (management) goes offline.
*   **Cost Efficiency:** By utilizing bare metal "Muscle" nodes, you achieve 10x the compute density of public cloud instances for a fraction of the cost.

---

### Use Cases

#### Secure Internal Tooling
*   **Scenario:** An analyst needs a dashboard to visualize sensitive logistics data.
*   **With ml.ink:** The analyst asks an AI agent to build it. The agent generates the code and deploys it to a secure, private subdomain (e.g., `logistics.internal.agency.gov`).
*   **Outcome:** Tool is live in minutes, not weeks, without data ever leaving the private network.

#### Isolated Research Sandboxes
*   **Scenario:** A research team needs to run untrusted open-source models or code.
*   **With ml.ink:** Agents provision ephemeral environments with strict resource caps and gVisor isolation.
*   **Outcome:** Safe experimentation without risking the wider network.

#### Rapid Prototyping on Bare Metal
*   **Scenario:** A large-scale simulation requires massive compute.
*   **With ml.ink:** Deploy instantly to high-performance dedicated servers (AMD EPYC / Threadripper) rather than expensive virtualized cloud instances.

---

### Technical Architecture Overview

**ml.ink** is not a "black box." It is a transparent orchestration layer over standard, battle-tested technologies.

| Layer | Technology | Purpose |
| :--- | :--- | :--- |
| **Interface** | **MCP (Machine Control Protocol)** | The API for AI Agents. |
| **Control** | **Temporal + Go** | Reliable, stateful workflow management. |
| **Build** | **Nixpacks** | reproducible builds from source code. |
| **Run** | **Docker + gVisor** | Secure, sandboxed container runtime. |
| **Network** | **Traefik + Sablier** | Edge routing and "scale-to-zero" efficiency. |

---

### Conclusion

**ml.ink** is the missing link between AI capability and infrastructure reality. It empowers your organization to harness the speed of AI development while maintaining the uncompromising security posture required by government and enterprise standards.

**Don't just let agents write code. Let them ship it—safely, on your terms.**
