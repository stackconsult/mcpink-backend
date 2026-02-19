# High-Density Container Orchestration

## Engineering Strategy & Implementation Plan

**Deploy MCP Platform — February 2026**
_Version 1.0 — Internal Engineering Reference_

---

This document captures comprehensive research into achieving Railway-class container density (10,000 processes per machine) on Hetzner dedicated infrastructure. It covers four alternative strategies, their tradeoffs, sequencing, code-level implementation details, and an honest economic analysis of when each approach is worth the engineering investment.

---

## 1. Why Density Matters — The Economic Case

Container density—the number of customer workloads we can run per physical server—is the single largest lever on our infrastructure margin. Our current architecture on Hetzner dedicated servers (AX102 at ~€65/month auction, AX162 at ~€150/month auction) gives us an infrastructure cost base that is **8–17x cheaper per server than Railway's colocation model**. However, Railway compensates by packing 10–20x more processes per machine.

The net effect: both platforms arrive at roughly similar cost-per-customer. But if we can close the density gap while keeping our Hetzner cost advantage, our margins become extraordinary.

### 1.1 Our Current Position

| Metric                     | 500 Customers | 1,000 Customers | 2,000 Customers | 5,000 Customers |
| -------------------------- | ------------- | --------------- | --------------- | --------------- |
| Infra cost/month           | $453          | $664            | $1,145          | $4,986          |
| Cost per customer          | $0.91         | $0.66           | $0.57           | $1.00           |
| Gross margin ($15 avg rev) | 94.0%         | 95.6%           | 96.2%           | 93.4%           |
| Pods per run node          | ~500          | ~500            | ~500            | ~500            |
| Total run nodes needed     | 3             | 5               | 10              | 25              |

These margins are already strong. The goal of this density work is not to survive—it's to **reduce the number of physical servers needed as we scale**, minimize blast radius of node failures, and delay the point where infrastructure complexity becomes our primary engineering concern.

### 1.2 The Railway Benchmark

Railway's CEO Jake Cooper claimed in a TechTarget interview during their $100M Series B announcement:

> "We've essentially built a version of Cloud Run or Lambda, plus the hardware that goes along with it, that allows us to get to a level of what we call ultra-density, where we can run 10,000 processes on a machine"

**Source:** TechTarget SearchCloudComputing, "Upstart cloud provider Railway turns heads with speed" (2025)
**Source:** VentureBeat, "Railway secures $100 million to challenge AWS with AI-native cloud" (2025)

**Important caveats on this claim:**

- This was a CEO quote during a fundraising press cycle, optimized for headlines
- "10,000 processes" almost certainly means most containers are idle—a typical hobby web app idles at 20–80MB RAM, near-zero CPU
- Railway has NOT disclosed: exact server specs, container runtime configuration, detailed architecture internals, or what percentage of those 10K are actively serving traffic
- **What Railway HAS confirmed (primary sources):** Temporal for orchestration (Railway blog: "So You Think You Can Scale?"), custom non-Kubernetes orchestrator (Jake Cooper LinkedIn), Podman for container runtime (Railway Help Station), eBPF networking stack, colocation hardware in 4+ regions (Railway blog: "So You Want to Build Your Own Data Center")

---

## 2. Container Isolation — Density vs. Security Tradeoff

The choice of container isolation technology is the most fundamental architectural decision affecting density. Stronger isolation costs more memory and CPU overhead per workload, directly reducing the number of containers per machine.

### 2.1 Isolation Technologies Compared

| Technology               | Memory Overhead                     | CPU Overhead               | Density Ceiling     | Security Level                        | Used By                       |
| ------------------------ | ----------------------------------- | -------------------------- | ------------------- | ------------------------------------- | ----------------------------- |
| Podman rootless          | ~0 (shared kernel)                  | None                       | 10,000+/machine     | Good (namespaces + cgroups + seccomp) | Railway                       |
| containerd (k3s default) | ~0 (shared kernel)                  | None                       | 10,000+/machine     | Good (same primitives as Podman)      | Most k8s clusters             |
| gVisor (runsc)           | 35–46Mi measured, 64Mi reserved     | 2–10x on syscall-heavy I/O | 1,500–3,000/machine | Strong (syscall interception)         | **Our platform**, Cloud Run   |
| Firecracker microVM      | ~128MB min per instance             | Near-native after boot     | Hundreds/machine    | Very strong (full VM isolation)       | AWS Lambda, Fly.io            |
| Kata Containers          | Similar to Firecracker              | Near-native                | Hundreds/machine    | Very strong (lightweight VM)          | OpenStack deployments         |

### 2.2 Our Decision: gVisor Sandboxing (Deployed)

All customer pods run under gVisor (`runtimeClassName: gvisor`). Every syscall is intercepted by a userspace kernel (the Sentry) — container code never touches the host kernel directly. This is the same isolation model used by Google Cloud Run.

**Security model:**

- **gVisor is the security boundary.** All syscalls hit the Sentry's emulated kernel, not the host. A container escape requires compromising the Sentry process itself — a much smaller attack surface than the full Linux kernel.
- **`allowPrivilegeEscalation: false`** — sets `no_new_privs` on the process (free, no compatibility cost).
- **No capability dropping.** Capabilities only affect gVisor's emulated kernel, not the host. Dropping caps breaks root-based images (nginx, postgres, redis need `CAP_SETUID`/`CAP_SETGID`). Same model as Railway.
- **Root allowed in pods.** The image's default user runs as-is. Root inside gVisor has no host privileges.
- **No seccomp, no AppArmor.** gVisor's syscall interception replaces both — adding them would be redundant and add no security benefit.

**Why gVisor over plain containerd hardening:** We run untrusted user code. Linux namespaces + seccomp + dropped caps is a defense-in-depth stack that still shares the host kernel — container escape CVEs are regular occurrences. gVisor eliminates this entire class of attack by moving the kernel to userspace. The density cost (~35–46Mi overhead per pod, measured Feb 2026) is modest relative to the security gain.

**Overhead details:** See `infra/README.md` § "gVisor memory overhead" for measured numbers, `kubectl top` accuracy notes, and systrap vs KVM tradeoffs. RuntimeClass reserves 64Mi per pod for the Sentry.

---

## 3. Four Strategies for High Density

We have identified four distinct approaches to increasing container density, each with different engineering cost, risk profile, and density ceiling. They are not mutually exclusive—Strategy 1 is the foundation for all others.

| Strategy                         | Density Ceiling                 | Engineering Time | Risk        | k8s Compatibility            | Best For                              |
| -------------------------------- | ------------------------------- | ---------------- | ----------- | ---------------------------- | ------------------------------------- |
| 1. Tuned k3s + Cilium + Longhorn | 800–2,500 pods/node             | 1–2 weeks        | Low         | Full k8s API                 | Immediate (0–2,000 customers)         |
| 2. Virtual Kubelet + containerd  | 5,000–10,000/node               | 4–6 weeks        | Medium      | kubectl works, some CSI gaps | If k8s per-pod overhead is bottleneck |
| 3. Temporal-native (bypass k8s)  | 10,000+/node                    | 6–8 weeks        | Medium-high | None on run nodes            | Railway-class density goal            |
| 4. Multi-node sharding           | 500–1,000/node, unlimited total | 0 (add servers)  | Low         | Full k8s API                 | Recommended scaling path              |

---

### 3.1 Strategy 1: Tuned k3s + Cilium + Longhorn

> **Status (Feb 2026):** Not deployed yet — planned as the next infrastructure step before launch. We currently use Flannel (k3s default) with per-namespace NetworkPolicy for tenant isolation. No Longhorn, no kubelet tuning beyond defaults. Cilium migration is a full cluster network reset (all pods restart), which has zero downtime cost pre-launch but would be disruptive with live customer traffic. Better to do it now.

**What:** Keep our existing k3s cluster, but replace Flannel with Cilium for networking, add Longhorn for volume replication, and tune kubelet parameters for higher pod density.

**Why this first:** Lowest risk, highest immediate impact. Unlocks 800–2,500 pods per node with full Kubernetes API compatibility. Every subsequent strategy builds on this foundation.

#### 3.1.1 Cilium Migration (Replace Flannel) — Future

**Current state:** Flannel with per-namespace `NetworkPolicy` objects (ingress-isolation + egress-isolation per namespace, see `infra/eu-central-1/k8s/templates/customer-namespace-template.yml`). This works correctly at current scale — Flannel delegates NetworkPolicy enforcement to kube-router, which programs iptables.

**Problem with Flannel + kube-router at scale:** Flannel delegates NetworkPolicy enforcement to kube-router, which programs iptables rules and uses ipsets for matching. Per-packet evaluation is O(1) (ipset lookups are hash-based), so packet forwarding itself is not the bottleneck. The real scaling costs are:

1. **`iptables-restore` atomic sync storms.** Every NetworkPolicy change triggers a full iptables-restore reload. At 1,000 namespaces (2,000 NetworkPolicy objects — ingress + egress per namespace), each sync takes 3–5 seconds. At 2,000+ namespaces, 5–10+ seconds. During the sync, the kernel holds a lock that can stall packet processing.
2. **2×N etcd objects from per-namespace policies.** Each namespace creates 2 NetworkPolicy objects. At 2,000 namespaces that's 4,000 objects that controllers must watch, reconcile, and sync. This adds etcd pressure and controller CPU load.
3. **kube-proxy iptables for service routing.** Separate from NetworkPolicy — kube-proxy maintains its own iptables chains for ClusterIP/NodePort routing, which also grow linearly with service count and trigger their own iptables-restore syncs.

**Why Cilium:** Cilium eliminates all three bottlenecks:

- **eBPF enforcement replaces iptables.** Policy changes update eBPF maps in-place — no iptables-restore sync, no kernel lock. Each namespace gets a numeric security identity; policy evaluation is an eBPF map lookup, O(1) regardless of namespace count.
- **One CiliumClusterwideNetworkPolicy for egress replaces N identical egress policies.** Ingress isolation ("allow from same namespace") is inherently namespace-scoped and still requires one `CiliumNetworkPolicy` per namespace — Cilium has no facility for a cluster-wide policy to reference "the same namespace as the matched endpoint" (GitHub Issue #24731, closed as "not planned"). This reduces policy objects from 2×N to N+1, and Cilium enforces the remaining N ingress policies via eBPF map lookups instead of iptables chain traversal.
- **eBPF service routing replaces kube-proxy.** Cilium's `kubeProxyReplacement=true` eliminates kube-proxy's iptables chains entirely.

**gVisor compatibility:** Cilium works with gVisor. Set `socketLB.hostNamespaceOnly=true` to bypass socket-level load balancing (gVisor's netstack can't use cgroup BPF hooks). Traffic falls back to TC BPF at the veth — works correctly. **Known risk:** There is a reported DNS resolution issue with `kubeProxyReplacement` + gVisor when pods are not using `hostNetwork` (gVisor issue #6998). Test DNS resolution from gVisor pods during migration before committing — if DNS fails, the workaround is `kubeProxyReplacement=false` with Cilium handling only NetworkPolicy enforcement.

**Identity cardinality at scale:** Cilium assigns a numeric security identity to each unique label set. The default identity map size is 16,384 (`--bpf-policy-map-max`). At 10,000 namespaces with varied labels, you'll approach this limit — each unique label combination per namespace gets a separate identity. Monitor identity count via `cilium identity list` and tune `--bpf-policy-map-max` upward if needed.

**Installation (k3s control node):**

```bash
# Reinstall k3s without Flannel
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="\
  --flannel-backend=none \
  --disable-network-policy \
  --disable=traefik \
  --cluster-cidr=10.42.0.0/16 \
  --service-cidr=10.43.0.0/16" sh -

# Install Cilium via Helm
helm install cilium cilium/cilium \
  --namespace kube-system \
  --set kubeProxyReplacement=true \
  --set k8sServiceHost=<CTRL_NODE_IP> \
  --set k8sServicePort=6443 \
  --set operator.replicas=1 \
  --set ipam.mode=cluster-pool \
  --set ipam.operator.clusterPoolIPv4PodCIDRList=10.42.0.0/16 \
  --set ipam.operator.clusterPoolIPv4MaskSize=24 \
  --set bpf.masquerade=true \
  --set enableIdentityMark=true \
  --set socketLB.hostNamespaceOnly=true  # Required for gVisor — bypasses cgroup BPF hooks
```

**Cluster-wide egress policy (replaces N identical per-namespace egress policies):**

Cilium has no facility for a cluster-wide policy to reference "the same namespace as the matched endpoint" — `{{namespace}}` is not valid Cilium syntax (GitHub Issue #24731, closed as "not planned"). Therefore ingress isolation must remain per-namespace. Egress rules are identical across all tenants and CAN be consolidated into a single cluster-wide policy.

```yaml
# ONE cluster-wide policy for egress (replaces N identical egress policies)
apiVersion: cilium.io/v2
kind: CiliumClusterwideNetworkPolicy
metadata:
  name: tenant-egress-isolation
spec:
  endpointSelector:
    matchLabels:
      tenant: "true"
  egress:
    - toCIDR:
        - 0.0.0.0/0
      toCIDRSet:
        - cidr: 0.0.0.0/0
          except:
            - 10.0.0.0/8
            - 172.16.0.0/12
            - 192.168.0.0/16
            - 169.254.169.254/32
    - toEndpoints:
        - matchLabels:
            k8s:io.kubernetes.pod.namespace: kube-system
            k8s-app: kube-dns
      toPorts:
        - ports:
            - port: "53"
              protocol: UDP
```

**Per-namespace ingress policy (still required — one per namespace):**

```yaml
# Created per namespace by the deployment pipeline (replaces K8s NetworkPolicy)
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata:
  name: ingress-isolation
  namespace: "<customer-namespace>"
spec:
  endpointSelector:
    matchLabels:
      tenant: "true"
  ingress:
    - fromEndpoints:
        - matchLabels:
            io.kubernetes.pod.namespace: "<customer-namespace>"
    - fromEndpoints:
        - matchLabels:
            app: traefik
```

This reduces policy objects from 2×N to N+1 (N ingress + 1 egress). The remaining N ingress policies are enforced via eBPF map lookups instead of iptables chain traversal — the enforcement speed improvement applies to all policies regardless of whether they're cluster-wide or namespaced.

#### 3.1.2 Longhorn for Volume Replication — Future

**Current state:** No distributed volume storage. Customer pods are stateless by design. Stateful data (git repos, registry images) lives on ops-1 with RAID1. Longhorn is only needed if we offer persistent volumes to customers.

**Problem it solves:** Hetzner auction servers can permanently die (reclaimed with 3-day notice, or hardware failure). Any volume data stored only on local NVMe is lost when the node dies. We need volumes to survive single-node failure for customers who use persistent storage.

**Why Longhorn over Ceph:** Longhorn is simpler to operate (no separate storage cluster), works as a standard Kubernetes CSI driver, and 2x replication across nodes is sufficient for our scale. Ceph is overkill until we exceed 20+ storage nodes.

```bash
helm install longhorn longhorn/longhorn \
  --namespace longhorn-system --create-namespace \
  --set defaultSettings.defaultReplicaCount=2 \
  --set defaultSettings.nodeDownPodDeletionPolicy=\
        delete-both-statefulset-and-deployment-pod
```

**Key setting:** `nodeDownPodDeletionPolicy=delete-both-statefulset-and-deployment-pod` ensures that when a node dies, Longhorn immediately makes volumes available on surviving nodes so pods can be rescheduled. Without this, StatefulSet pods hang in Terminating state waiting for the dead node.

**Storage overhead:** Longhorn runs 5+ system pods per node (manager, driver, CSI components). This eats into density. At 2x replication, every GB of customer data consumes 2GB of actual NVMe. Plan for this in node capacity budgets.

#### 3.1.3 Kubelet Tuning Parameters — Future

**Current state:** Default kubelet settings (max-pods=110). Sufficient at current scale.

**Run node config (`/etc/rancher/k3s/config.yaml`):**

```yaml
kubelet-arg:
  - "max-pods=800" # Default is 110; 800 is safe ceiling
  - "pods-per-core=0" # Disable per-core limits
  - "kube-api-qps=100" # Default 5; needed for fast status updates
  - "kube-api-burst=200" # Default 10
  - "serialize-image-pulls=false" # Parallel image pulls
  - "node-status-update-frequency=30s" # Reduce API chatter
```

**Control node tuning:**

```yaml
kube-controller-manager-arg:
  - "node-monitor-grace-period=20s" # Default 40s; faster failure detection
  - "pod-eviction-timeout=30s" # Default 5m; faster rescheduling
```

**Evidence for 800–2,500 pods/node ceiling:** Red Hat published results achieving 2,500 pods per node on OpenShift 4.13 (RHEL CoreOS, CRI-O runtime). They found all densities up to 2,500 worked, but 2,750 caused pod malfunction due to network stack saturation (OVS socket errors). Note: with gVisor, each pod carries ~64Mi overhead (Sentry), so the practical ceiling on our 256GB run-1 is lower than bare containerd. _Source: Red Hat Engineering Blog, "Running 2500 pods per node on OCP 4.13" (November 2025)._

Industry research confirms 600–800 pods per node is practical on well-tuned clusters without instability, while 1,000+ requires careful monitoring. _Source: Kubernetes maxPods guide, CopyProgramming (November 2025)._

---

### 3.2 Strategy 2: Virtual Kubelet + containerd Direct

**What:** Replace the standard kubelet on run nodes with a Virtual Kubelet provider that talks directly to containerd, bypassing kubelet's per-pod overhead. The k3s control plane still sees the run node as a Kubernetes node and can schedule pods to it via kubectl.

**Why:** The kubelet itself adds overhead per pod: pause container, pod sandbox creation, cgroup hierarchy management, per-pod iptables rules. Virtual Kubelet eliminates all of this while keeping the Kubernetes API as the interface.

#### 3.2.1 Architecture

```
k3s-1 (ctrl) — unchanged, sees run-1 as a virtual node

run-1 — NO real kubelet, NO kube-proxy
  ├── Virtual Kubelet (custom Go binary, ~2,000 lines)
  │   ├─ Registers as k8s node with capacity: 10,000 pods
  │   ├─ Receives pod assignments from API server
  │   ├─ Translates to containerd API calls
  │   └─ Reports pod status back to API server
  ├── containerd (5,000–10,000 containers)
  ├── Traefik (reads Ingress objects from k3s API)
  └── Cilium agent (eBPF networking)
```

#### 3.2.2 Core Provider Implementation (Go)

```go
type DeployMCPProvider struct {
    containerdClient *containerd.Client
    temporalClient   client.Client
    containers       sync.Map  // podKey -> container
    volumes          *VolumeManager
}

func (p *DeployMCPProvider) CreatePod(ctx context.Context,
    pod *v1.Pod) error {
    image, _ := p.containerdClient.Pull(ctx,
        pod.Spec.Containers[0].Image)
    container, _ := p.containerdClient.NewContainer(ctx, containerID,
        containerd.WithImage(image),
        containerd.WithNewSnapshot(containerID+"-snap", image),
        containerd.WithNewSpec(
            oci.WithProcessArgs(cmd...),
            oci.WithLinuxNamespace(specs.LinuxNamespace{
                Type: specs.NetworkNamespace,
                Path: netNS.Path(),
            }),
            oci.WithNoNewPrivileges,
            oci.WithDroppedCapabilities(allCaps...),
            oci.WithSeccompProfile(defaultProfile),
            oci.WithUser("1000:1000"),
        ),
    )
    task, _ := container.NewTask(ctx, cio.NewCreator())
    task.Start(ctx)
    return nil
}
```

#### 3.2.3 What You Keep vs. Lose

| Capability           | Status with Virtual Kubelet            | Workaround                                                                |
| -------------------- | -------------------------------------- | ------------------------------------------------------------------------- |
| kubectl get pods     | Works — VK reports pod status          | None needed                                                               |
| kubectl logs         | Must implement in provider             | Stream from containerd task                                               |
| Traefik Ingress      | Works — reads from k8s API             | None needed                                                               |
| cert-manager         | Works — runs on ctrl node              | None needed                                                               |
| Longhorn CSI volumes | **BROKEN** — no real kubelet to attach | Option A: mount iSCSI manually in provider; Option B: local NVMe + backup |
| k8s health checks    | Must reimplement in provider           | containerd exec + HTTP check                                              |
| k8s rolling updates  | Must reimplement in provider           | Temporal workflow handles this already                                    |
| HPA autoscaling      | Needs custom metrics                   | Temporal-driven scaling preferred                                         |

**Volume handling is the critical gap.** Longhorn CSI requires a real kubelet to handle iSCSI target attachment. With Virtual Kubelet, you must either (a) implement iSCSI attachment in your provider (~500 lines of Go), (b) use local NVMe with daily backups to Hetzner Storage Box, or (c) keep Longhorn only on nodes with real kubelets and route stateful pods there.

**Estimated effort:** 4–6 weeks for a senior Go engineer. The Virtual Kubelet framework provides the node registration and API plumbing. You implement ~2,000 lines of provider code covering CreatePod, DeletePod, GetPod, GetPodStatus, GetContainerLogs, plus the volume and networking layers.

**Reference:** `github.com/virtual-kubelet/virtual-kubelet` (Go library), `github.com/virtual-kubelet/cri` (reference CRI provider implementation)

---

### 3.3 Strategy 3: Temporal-Native Orchestration (Bypass k8s Entirely)

**What:** Remove Kubernetes from run nodes entirely. The deployer-worker Temporal activity talks directly to a lightweight gRPC agent on each run node, which manages containerd, networking, and volumes. This is architecturally identical to what Railway built.

#### 3.3.1 Architecture Change

```
CURRENT FLOW:
  Temporal workflow → deployer-worker → kubectl apply YAML
    → k3s API server → kubelet → containerd

NEW FLOW:
  Temporal workflow → deployer-worker → run-node agent (gRPC)
    → containerd (container lifecycle)
    → Traefik file provider (routing config)
    → nftables (network isolation)
    → local NVMe volume manager
    → status → Postgres (state of the world)
```

#### 3.3.2 The Run-Node Agent (~1,000–3,000 Lines Go)

This is the only net-new binary needed. Everything it calls already exists on the node:

| Concern             | Existing Component                  | Integration Method                                       |
| ------------------- | ----------------------------------- | -------------------------------------------------------- |
| Container lifecycle | containerd                          | Go client: `github.com/containerd/containerd/v2`         |
| Image pulling       | containerd                          | `containerd.Pull()` with registry auth                   |
| Networking          | CNI plugins + nftables              | Same CNI containerd already uses, nftables for isolation |
| Ingress routing     | Traefik                             | File provider: write YAML config files per service       |
| DNS                 | CoreDNS                             | Standalone or systemd-resolved with custom zones         |
| TLS                 | cert-manager (ctrl) or Traefik ACME | Unchanged                                                |
| Volume management   | Local NVMe                          | Bind-mount directories with xfs_quota                    |
| Health checks       | containerd exec                     | HTTP/TCP probes via agent goroutines                     |
| Logs                | containerd task I/O                 | Stream to Loki via promtail or direct push               |
| Metrics             | cAdvisor or cgroup reads            | Expose to Prometheus                                     |

#### 3.3.3 Networking Without Kubernetes

The run-node agent handles namespace isolation via per-namespace network bridges and nftables rules:

- Each customer namespace gets its own Linux bridge (e.g., `br-<namespace-hash>`)
- Containers in the same namespace share the bridge and can communicate freely
- Containers in different namespaces are on different bridges—fully isolated at L2
- nftables rules: allow egress to public internet, block private IP ranges (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.169.254/32), allow DNS to CoreDNS
- Traefik reads per-service config files (file provider) for ingress routing—no Kubernetes Ingress objects needed

**Railway's approach for reference:** Railway assigns each container a unique IPv6 address and pushes it to their network control plane via gRPC, which propagates to all proxies and DNS servers within milliseconds. We can achieve the same with simpler tooling given our smaller scale. _Source: Railway blog, "So You Think You Can Scale?"_

#### 3.3.4 What You Gain vs. Lose

| Category      | Gain                                                   | Loss                                                           |
| ------------- | ------------------------------------------------------ | -------------------------------------------------------------- |
| Density       | 10,000+ containers per node (no k8s overhead)          | Cannot use any k8s tooling on run nodes                        |
| Startup speed | ~500ms container start (vs. 3–5s with k8s scheduling)  | No kubectl on run nodes for debugging                          |
| Visibility    | Full Temporal workflow visibility for every deployment | Must build custom log/metrics pipeline                         |
| Complexity    | Run nodes are just containerd + agent (very simple)    | Agent is custom code you own and maintain                      |
| Scheduling    | Trivial: pick node with most available resources       | No k8s scheduler intelligence (affinity, priority, preemption) |

**Estimated effort:** 6–8 weeks for a senior Go engineer. The agent is simpler than the Virtual Kubelet provider because you don't need to conform to the Kubernetes API. But you own more: scheduling, health checks, log routing, metrics collection.

**Difficulty:** 7/10. The individual pieces are straightforward. The challenge is the integration surface: Temporal workflow → gRPC → containerd → Traefik → DNS → nftables all need to be correctly wired and tested for edge cases (container OOM, node reboot, image pull failure, volume full).

---

### 3.4 Strategy 4: Multi-Node Horizontal Sharding (Recommended Default)

**What:** Don't pursue extreme per-node density. Instead, keep pods at 500–800 per node and add more cheap Hetzner servers as needed. This is not a cop-out—it's the economically rational path at Hetzner prices.

#### 3.4.1 The Math That Makes This Work

At Hetzner auction pricing (€65/month per AX102 server, ~$70/month), the cost of adding a server is negligible compared to the engineering cost of increasing per-node density:

| Scale                          | Run Nodes Needed | Monthly Cost | Cost of 1 Engineer Week Instead |
| ------------------------------ | ---------------- | ------------ | ------------------------------- |
| 1,000 customers (2,000 pods)   | 5 nodes @ $70    | $350/month   | ~$3,000–$5,000 (one-time)       |
| 2,000 customers (4,000 pods)   | 10 nodes @ $70   | $700/month   | ~$3,000–$5,000 (one-time)       |
| 5,000 customers (10,000 pods)  | 25 nodes @ $70   | $1,750/month | ~$3,000–$5,000 (one-time)       |
| 10,000 customers (20,000 pods) | 50 nodes @ $70   | $3,500/month | ~$3,000–$5,000 (one-time)       |

**Key insight:** Even at 10,000 customers, our entire run-node fleet costs $3,500/month. A single week of an engineer's time working on density optimization costs more than an entire month of infrastructure. The breakeven point where per-node density optimization pays for itself is somewhere beyond 20,000–50,000 customers.

#### 3.4.2 Resilience Advantage

Spreading pods across more nodes dramatically improves failure recovery:

| Scenario                           | High Density (2,500/node, 4 nodes) | Horizontal (500/node, 20 nodes) |
| ---------------------------------- | ---------------------------------- | ------------------------------- |
| Node dies: pods lost               | 2,500 pods                         | 500 pods                        |
| Scheduling storm                   | 25–125 seconds just for decisions  | ~5 seconds                      |
| Image pull storm                   | Hundreds of GB, saturates network  | Moderate, network handles it    |
| Longhorn volume rebuild            | Massive network saturation         | Minimal impact                  |
| Recovery time                      | 5–15 minutes degraded service      | 30–60 seconds                   |
| Pods per surviving node (absorbed) | ~833 each (potentially overloaded) | ~26 each (trivial)              |

**The fundamental problem with high density:** If you pack 2,500 pods on one node and that node dies (Hetzner auction servers CAN permanently die—reclaimed with 3-day notice or hardware failure), you face a cascading failure: 2,500 pods try to reschedule simultaneously, causing a scheduling storm, image pull storm, Longhorn volume rebuild storm, Cilium eBPF programming backlog, and Traefik route update storm. Some pods may never reschedule if remaining nodes can't absorb the capacity.

**Planning for failover spare nodes:** Budget 1 spare run node per 4 active nodes. When a node dies, its 500 pods spread across the remaining nodes (~26 extra pods each—trivial). The spare ensures you never run at >80% capacity.

---

## 4. Namespace-per-Customer Scaling Limits

Our architecture uses a Kubernetes namespace per customer project for isolation. This model has specific scaling implications that are separate from pods-per-node density.

| Customer Count | Namespaces | Key Challenge                                                    | Mitigation                                                                  |
| -------------- | ---------- | ---------------------------------------------------------------- | --------------------------------------------------------------------------- |
| 100            | 100        | None — k3s handles fine                                          | N/A                                                                         |
| 500            | 500        | NetworkPolicy count if using per-namespace policies              | Cilium: consolidate egress to one cluster-wide policy (N+1 vs 2×N objects)  |
| 1,000          | 1,000      | etcd/SQLite watch pressure; all controllers watch all namespaces | Cilium eBPF identities; tune API server QPS                                 |
| 2,000          | 2,000      | API server latency; Traefik Ingress watch across all namespaces  | Consider namespace-scoped Traefik instances                                 |
| 5,000          | 5,000      | k3s SQLite may degrade; Longhorn replica count increases         | Switch to embedded etcd HA (`k3s --cluster-init`)                           |
| 10,000+        | 10,000+    | Single cluster is at its limit                                   | Shard: multiple k3s clusters per region, Temporal routes to correct cluster |

**Critical:** The real ceiling for namespace-per-customer is not pods-per-node. It's etcd (or SQLite) state. Each namespace creates: 1 Namespace object, 1 ServiceAccount, 1 Secret, 1 ResourceQuota, plus N Deployments, N Services, N Ingresses. At 5,000 namespaces with 2 services each, that's ~35,000 objects in etcd. This is manageable but needs monitoring.

**k3s SQLite vs etcd HA:** k3s uses SQLite by default, which is actually fine for single ctrl node up to ~5,000 namespaces. At 3,000+ namespaces, consider switching to embedded etcd HA for write throughput:

```bash
# k3s with embedded etcd HA (on ctrl nodes)
curl -sfL https://get.k3s.io | sh -s - server --cluster-init
# Second ctrl node joins:
curl -sfL https://get.k3s.io | sh -s - server \
  --server https://<FIRST_CTRL>:6443
```

**Never replace k3s with k8s.** k3s IS k8s (CNCF certified conformant). The only differences are packaging: single binary, SQLite default, 512MB ctrl overhead vs k8s's multiple binaries, required etcd, 1.5–2GB overhead. k3s supports etcd HA natively. There is zero reason to switch to upstream Kubernetes.

---

## 5. Hardware Recommendations

### 5.1 Server Selection for Density

| Server              | Specs                                              | Price (auction est.) | Best Role               | Density Notes                             |
| ------------------- | -------------------------------------------------- | -------------------- | ----------------------- | ----------------------------------------- |
| Hetzner AX102       | Ryzen 9 7950X3D, 16c/32t, 128GB RAM, 2×1.92TB NVMe | ~€65/month           | Run node (primary)      | 128GB ≈ 1,600 containers at 80MB avg idle |
| Hetzner AX162-R     | EPYC 9454P, 48c/96t, 256GB RAM, 2×1.92TB NVMe      | ~€150/month          | Run node (high density) | 256GB ≈ 3,200 containers at 80MB avg idle |
| AX162 + RAM upgrade | Same CPU, 512GB RAM                                | +~€100/month         | Maximum density target  | 512GB ≈ 6,400 containers at 80MB avg idle |
| CX32 (cloud)        | 4 vCPU, 16GB RAM                                   | ~€18/month           | ctrl node               | Runs k3s API server + etcd                |
| CCX23 (cloud)       | 4 dedicated CPU, 16GB RAM                          | ~€40/month           | Build node              | BuildKit image builds                     |

**Key principle: RAM is the binding constraint for density, not CPU.** A typical idle web app container uses 30–80MB of RAM and near-zero CPU. Railway's 10,000-container claim on high-end servers implies ~512GB–1TB RAM with most containers idle. CPU cores matter only for active request handling, which is bursty.

**Railway's hardware (inferred, not disclosed):** High core-count AMD EPYC (96–128 cores), 512GB–1TB RAM, NVMe SSDs. They optimized for power density—performance per watt matters because power is their largest colo cost, paid as a fixed monthly commit per kW. _Source: Railway blog and Jake Cooper podcast (The Split)._

---

## 6. Recommended Implementation Sequence

### Phase 0: Current State (Deployed)

What's actually running as of Feb 2026:

- **Container isolation:** gVisor (`runtimeClassName: gvisor`) on all customer pods. Systrap platform, 64Mi overhead reserved per pod.
- **Networking:** Flannel (k3s default) with per-namespace NetworkPolicy objects (ingress-isolation + egress-isolation). Works at current scale.
- **Storage:** No distributed volumes. Customer pods are stateless. Stateful data on ops-1 (RAID1).
- **Kubelet:** Default settings (max-pods=110).
- **Nodes:** Single run node (run-1, 256GB EPYC). ~500 pods comfortable ceiling at current density.
- **Load balancing:** Hetzner LB (TCP passthrough) → run-1 for custom domains. Cloudflare LB for `*.ml.ink`.

### Phase 1: Network & Density Optimization — PRE-LAUNCH INFRASTRUCTURE HARDENING

- **Migrate Flannel → Cilium** on all nodes. Install `CiliumClusterwideNetworkPolicy` for egress; replace per-namespace K8s `NetworkPolicy` with per-namespace `CiliumNetworkPolicy` for ingress (N+1 objects vs 2×N). Include `socketLB.hostNamespaceOnly=true` for gVisor compatibility. Test DNS resolution from gVisor pods before committing (gVisor issue #6998).
- **Install Longhorn** with 2x replication (only if offering persistent volumes to customers).
- **Tune kubelet:** max-pods=800, serialize-image-pulls=false, kube-api-qps=100. Tune controller: node-monitor-grace-period=20s, pod-eviction-timeout=30s.
- **Label all customer pods** with `tenant: "true"` and ensure namespace creation workflow adds the label.

**Trigger:** Pre-launch — Cilium migration is a full cluster network reset (all pods restart). Zero downtime cost now; would be disruptive with live customer traffic.
**Outcome:** Eliminates iptables as the first scaling bottleneck: no more iptables-restore sync storms, consolidates N egress policies into one cluster-wide policy (N+1 total vs 2×N), replaces kube-proxy with eBPF service routing. The etcd/API server ceiling (~3,000–5,000 namespaces on k3s SQLite) is unchanged — Cilium improves enforcement speed, not control plane capacity. Supports 800–1,500 pods per run node.
**Risk:** Low. Cilium is battle-tested. Longhorn is stable. These are configuration changes, not code changes.

### Phase 2: Scale Horizontally (Month 2–6) — AS CUSTOMERS GROW

- **Add run nodes on demand.** One AX102 auction server per 500 new customers. Order takes hours, Ansible provisions in minutes.
- **Target 500–800 pods per node** for resilience. Budget 1 spare per 4 active nodes.
- **Monitor etcd/SQLite health.** Watch API server latency, etcd DB size, controller reconciliation time.
- **At 3,000+ namespaces,** evaluate embedded etcd HA (`k3s --cluster-init` with 2–3 ctrl nodes).

**Outcome:** Scales to 5,000+ customers across ~25 run nodes. Total infra cost ~$1,750/month.
**Risk:** Very low. Adding servers is a solved problem.

### Phase 3: Density Optimization (Month 6–12) — ONLY IF NEEDED

**Gate:** Only begin this phase if one of: (a) node count exceeds 50 and management overhead is significant, (b) Hetzner auction stock is limited for your preferred server type, (c) you have a specific customer need for sub-second container start.

- **Option A — Virtual Kubelet:** Build the containerd provider. 4–6 weeks. Increases ceiling to 5,000–10,000 containers per node. Keeps kubectl.
- **Option B — Temporal-native agent:** Build the gRPC run-node agent. 6–8 weeks. Removes k8s from run nodes entirely. Maximum density but you own the full orchestration stack.
- **Option C — Cluster sharding:** Deploy multiple k3s clusters per region, each handling 2,000–3,000 namespaces. Temporal task queues route deployments to the correct cluster. 0 weeks of density engineering, just infrastructure provisioning.

**Recommendation:** Option C (sharding) is almost always the right choice at this stage. It requires no new code, works with all existing tooling, and is how most cloud providers scale internally. Pursue Option A or B only if you have a specific technical reason that sharding doesn't solve.

### Phase 4: Railway-Class Density (Month 12+) — MAYBE NEVER

**Gate:** Only if you're serving 20,000+ customers, running 100+ servers, and the marginal cost savings of higher density per node would fund the engineering team needed to build and maintain a custom orchestrator.

- **Full Temporal-native architecture** with custom run-node agent, containerd direct, nftables isolation, Traefik file provider, local NVMe volume management.
- **Container start time drops to ~500ms** (vs. 3–5s with k8s scheduling overhead)
- **Density reaches 10,000+ per node** (matching Railway's claim)

**The honest assessment:** At Hetzner prices, this phase may never be economically justified. The engineering cost of building and maintaining a custom orchestrator (8+ weeks to build, ongoing maintenance forever) only pays for itself when infrastructure savings exceed $50K–$100K/year. That requires 20,000+ customers. Your time is better spent getting customers.

---

## 7. Complete Component Map

Every piece needed for Railway-class density already exists as open source or is already deployed in our stack:

| Concern                 | Existing Component           | Status                          | Integration                                      |
| ----------------------- | ---------------------------- | ------------------------------- | ------------------------------------------------ |
| Workflow orchestration  | Temporal                     | Deployed ✓                      | deployer-worker already handles deploy lifecycle |
| Container runtime       | containerd                   | Deployed ✓ (via k3s)            | Go client: `containerd/containerd/v2`            |
| Container sandbox       | gVisor (runsc)               | **Deployed ✓**                  | `runtimeClassName: gvisor` on all customer pods  |
| Container lifecycle API | containerd Go client         | Available                       | Used by Virtual Kubelet or custom agent          |
| k8s bridge (optional)   | Virtual Kubelet              | Available                       | `github.com/virtual-kubelet/virtual-kubelet`     |
| Networking (current)    | Flannel + NetworkPolicy      | **Deployed ✓**                  | Per-namespace ingress/egress policies            |
| Networking (future)     | Cilium                       | Not deployed — planned pre-launch (Phase 1) | Replaces Flannel + kube-router + kube-proxy with eBPF |
| Networking (post-k8s)   | CNI plugins + nftables       | Available on all nodes          | Same CNI containerd already uses                 |
| Ingress routing         | Traefik                      | Deployed ✓                      | k8s Ingress OR file provider mode                |
| Image building          | BuildKit + Railpack          | Deployed ✓                      | Unchanged across all strategies                  |
| Image storage           | Registry v2                  | Deployed ✓                      | Unchanged                                        |
| DNS                     | CoreDNS                      | Deployed ✓                      | Can run standalone or in k8s                     |
| TLS certificates        | cert-manager + Let's Encrypt | Deployed ✓                      | Unchanged                                        |
| Volume storage          | Local NVMe                   | Deployed ✓ (stateless pods)     | No distributed volumes; Longhorn is future       |
| Observability           | Prometheus + Loki + Grafana  | Deployed ✓                      | Unchanged                                        |
| Load balancing          | Cloudflare + Hetzner LB      | Deployed ✓                      | CF for *.ml.ink, Hetzner LB for custom domains   |
| Config management       | Ansible                      | Deployed ✓                      | Add roles for Cilium, Longhorn when needed       |
| Customer databases      | Turso                        | Deployed ✓                      | External, unaffected by density changes          |
| Auth                    | Firebase + GitHub OAuth      | Deployed ✓                      | Unchanged                                        |

**The single net-new component** (only needed for Strategy 2 or 3) is a run-node agent binary: ~2,000–3,000 lines of Go that wires containerd + networking + volumes + health checks + metrics. Everything it calls already exists on the node.

---

## 8. Primary Sources & References

### Railway Architecture (Confirmed Facts)

- **10,000 processes/machine claim:** Jake Cooper, TechTarget SearchCloudComputing, "Upstart cloud provider Railway turns heads with speed" (2025)
- **Same claim:** VentureBeat, "Railway secures $100 million to challenge AWS with AI-native cloud" (2025)
- **Temporal orchestration, horizontal scaling:** Railway blog, "So You Think You Can Scale?" (September 2023)
- **Physical data center build:** Railway blog, "So You Want to Build Your Own Data Center" (January 2025)
- **Podman container runtime:** Railway Help Station, runtime-security-a4ce158a
- **Custom non-Kubernetes orchestrator:** Jake Cooper LinkedIn ("evolving an internal orchestrator because Kube won't scale")
- **Network metering, peering economics:** The Split podcast, "Solving the Hardest Problems in Dev Tools" with Jake Cooper (2025)
- **Railway Metal documentation:** docs.railway.com/railway-metal

### Kubernetes Density Research

- **2,500 pods/node on OpenShift 4.13:** Red Hat Engineering Blog, "Running 2500 pods per node on OCP 4.13" (November 2025). Found 2,750 caused OVS socket errors.
- **600–800 pods/node practical ceiling:** Kubernetes maxPods guide, CopyProgramming (November 2025). 1,000+ causes significant instability.
- **Virtual Kubelet project:** `github.com/virtual-kubelet/virtual-kubelet`. CRI reference provider: `github.com/virtual-kubelet/cri`
- **Cilium eBPF networking:** cilium.io documentation. Security identities, ClusterwideNetworkPolicy.
- **Longhorn distributed storage:** longhorn.io. CSI driver, replica management, node failure handling.

### Container Isolation

- **Podman rootless + seccomp hardening:** hardenedlinux.org, "Container Hardening Process" (October 2024)
- **Container security mechanisms:** SUSE blog, "Demystifying Containers Part IV: Container Security" (March 2020)
- **gVisor overhead analysis:** Google Cloud documentation; community benchmarks showing 2–10x syscall overhead
- **Firecracker microVM:** AWS open source, used by Lambda and Fly.io. ~128MB minimum per instance.

### Economic Analysis

- **Hetzner AX102 pricing:** hetzner.com/dedicated-rootserver. List: €128/month. Auction: ~€65/month typical (varies).
- **Hetzner AX162 pricing:** List: €246/month (Germany). Auction: ~€150/month typical.
- **Railway pricing:** railway.com/pricing. vCPU: $20/month. Memory: $10/GB/month. Network: $0.05/GB. Volume: $0.15/GB/month.
- **Colocation cost estimates:** Industry standard power pricing ~$163/kW, 3–5kW per server. Space/cage: ~$100–500/month. Cross-connects: $200–500/month each.
- **Temporal deployment patterns:** Chronosphere blog, "How Chronosphere built a deployment system with Temporal" (September 2025)

---

## 9. Decision Framework

Use this flowchart to decide which strategy to invest in at any given time:

**Question 1: Are you under 2,000 customers?**
→ Yes: Strategy 1 (tuned k3s + Cilium + Longhorn) + Strategy 4 (add servers). Stop here.

**Question 2: Are you at 2,000–5,000 customers and node count is manageable (<30)?**
→ Yes: Keep adding servers (Strategy 4). Monitor etcd health. Consider cluster sharding at 3,000+ namespaces.

**Question 3: Are you at 5,000+ customers and hitting k3s API server bottleneck?**
→ Shard clusters first (Strategy 4 variant). Multiple k3s clusters, Temporal routes to correct one.

**Question 4: Is per-node density specifically the bottleneck (not API server, not etcd)?**
→ Unlikely, but if yes: Strategy 2 (Virtual Kubelet) or Strategy 3 (Temporal-native).

**Question 5: Is sub-second container startup a competitive requirement?**
→ Strategy 3 only. This is Railway's competitive moat, but requires owning the full orchestration stack.

---

**The single most important takeaway:** Don't chase 10,000 pods per node. Chase 500 pods per node across many cheap nodes. At Hetzner auction prices (€65/month per server), density optimization is almost always less valuable than spending the same engineering time getting more customers. Build the custom orchestrator only when customer count makes the infrastructure savings pay for the engineering.
