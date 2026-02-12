# Future Architecture

> Architecture decisions and scaling progression for Deploy MCP infrastructure.

---

## Table of Contents

1. [Scaling Progression](#scaling-progression)
2. [Cluster Abstraction](#cluster-abstraction)
3. [Storage Strategy](#storage-strategy)
4. [Provider Abstractions](#provider-abstractions)
5. [Volume Feature Design](#volume-feature-design)
6. [Multi-Cluster Architecture](#multi-cluster-architecture)
7. [Operational Concerns](#operational-concerns)
8. [What NOT to Abstract](#what-not-to-abstract)
9. [Railway Learnings](#railway-learnings)

---

## Scaling Progression

### Stage 1: Now → ~5K customers

**Stack:** k3s (single cluster) + Longhorn

- Single k3s cluster, 3-10 run nodes (Hetzner dedicated, 256GB RAM each)
- Longhorn for volume replication across run nodes
- CX32 ctrl node is the first bottleneck — upgrade to CX52 or dedicated early
- ~10 etcd objects per customer (pods + PVCs + services + ingresses + secrets)
- CX32 (8GB) comfortable at ~1000 objects, tight at ~2000-3000

**Capacity per run node (256GB RAM):**

```
256GB total
  - 2GB    OS / kubelet / system
  - 0.5GB  Traefik (DaemonSet)
  - 3GB    Longhorn base (manager, driver, CSI)
  ─────────
~250GB available for customer workloads + volume replicas
```

**Action items:**

- Deploy Longhorn scoped to `pool=run` nodes only
- Enforce volume size limits from day 1
- Implement volume backups to S3/R2 from day 1
- Upgrade ctrl node before adding more run nodes

### Stage 2: ~5K → ~20K customers

**Stack:** k3s HA (multi-master) or RKE2, multi-cluster, Longhorn or transition to Ceph

- Shard by region: `eu-west-1`, `us-east-1`
- 3 master nodes per cluster (HA control plane with embedded etcd)
- Product API routes customers to clusters
- Longhorn if <500 volumes per cluster, Ceph if more
- Temporal + product API stay centralized on Railway
- One deployer-worker per cluster

**Action items:**

- Implement cluster routing in product API
- Automate cluster provisioning (Terraform/Ansible)
- Evaluate Ceph for new clusters (don't migrate old ones)

### Stage 3: ~20K → ~100K customers

**Stack:** Multi-cluster k8s + Ceph + custom scheduling

- Kubernetes still the container runtime
- Smart placement logic above k8s (which cluster has capacity?)
- Ceph for storage (scales better than Longhorn at thousands of volumes)
- Dedicated build clusters per region
- Possibly dedicated storage nodes separate from compute nodes

### Stage 4: 100K+ customers

**Stack:** Re-evaluate everything

- Most companies never need to leave Kubernetes at this scale (Shopify runs 600K+ pods on k8s)
- Railway left k8s for sub-second deploys and extreme scheduling control — that's a product decision, not a scale limitation
- New clusters get whatever storage/orchestration makes sense at that time

### Kubernetes Hard Limits (reference)

| Limit              | Upstream tested |
| ------------------ | --------------- |
| Nodes per cluster  | 5,000           |
| Pods per cluster   | 150,000         |
| Total etcd objects | 300,000         |

Performance degrades well before these limits. Plan to shard around 500-1000 nodes / 50-100K pods.

---

## Cluster Abstraction

### Cluster as a First-Class Entity

Clusters are a database entity, not a hardcoded config. Even with one cluster, all code resolves cluster config dynamically.

```sql
CREATE TABLE clusters (
    id               TEXT PRIMARY KEY,              -- "eu-west-1"
    name             TEXT NOT NULL,                  -- "Europe West 1"
    region           TEXT NOT NULL,                  -- "eu-west"
    task_queue        TEXT NOT NULL UNIQUE,           -- "deployer-eu-west-1"
    api_endpoint     TEXT,                           -- k8s API endpoint
    storage_provider TEXT NOT NULL DEFAULT 'longhorn', -- "longhorn" | "rook-ceph"
    registry_url     TEXT NOT NULL,                  -- "10.0.1.4:5000"
    status           TEXT NOT NULL DEFAULT 'active', -- active | draining | maintenance
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Services reference their cluster:

```sql
ALTER TABLE services ADD COLUMN cluster_id TEXT NOT NULL
    REFERENCES clusters(id) DEFAULT 'eu-west-1';
```

### Task Queue Routing

Temporal task queues are postfixed with cluster ID. The deployer-worker reads its cluster assignment from environment config and registers against the corresponding queue.

```go
// Product API: resolve task queue from cluster
cluster := getClusterForService(service)
workflow.ExecuteActivity(ctx, activities.Deploy, workflow.ActivityOptions{
    TaskQueue: cluster.TaskQueue, // "deployer-eu-west-1"
})
```

```go
// cmd/deployer-worker/main.go
// Same binary, different cluster assignment per instance
w := worker.New(temporalClient, cfg.ClusterTaskQueue, worker.Options{})
// env: CLUSTER_TASK_QUEUE=deployer-eu-west-1
```

### Cluster Placement

Today: single cluster, trivial. Future: placement logic selects cluster based on region preference, capacity, and volume requirements.

```go
cluster := selectCluster(ctx, SelectClusterInput{
    Region:    user.PreferredRegion,
    HasVolume: len(volumes) > 0,
})
```

---

## Storage Strategy

### Decision: Longhorn Now, Ceph on New Clusters Later

**Longhorn** is the right choice for Stage 1-2:

- Built for k3s, first-class support
- Simple failure modes (volume degraded → replica missing → rebuild)
- Handles up to ~500 volumes per cluster comfortably
- Synchronous replication across run nodes
- Auto-rebuilds replicas when nodes return

**Rook-Ceph** for Stage 2+ on new clusters:

- Fixed daemon overhead regardless of volume count (scales to 10,000+ volumes)
- Better IOPS (kernel RBD client vs Longhorn's iSCSI)
- Dedicated storage node architecture
- More powerful snapshots, clones, mirroring
- Complex failure modes (PG states, CRUSH maps, OSD trees)

### Migration Strategy: No Big Bang

When Ceph makes sense, deploy it on **new clusters only**. Old clusters stay on Longhorn until they naturally wind down. Multi-cluster means no forced migration — stop putting new customers on old clusters and let them drain organically.

### Longhorn Configuration

Scoped exclusively to run nodes to protect ctrl, ops, and build pools:

```yaml
# Helm values for Longhorn
longhornManager:
  nodeSelector:
    pool: run
  tolerations: [] # Remove default tolerate-all

longhornDriver:
  nodeSelector:
    pool: run

defaultSettings:
  systemManagedComponentsNodeSelector: "pool:run"
  defaultDataLocality: "best-effort"
  defaultReplicaCount: 2 # 3 when we have 3+ run nodes
```

### Storage Comparison Reference

|                         | Longhorn              | Rook-Ceph                       |
| ----------------------- | --------------------- | ------------------------------- |
| Min nodes               | 2                     | 3 (MON quorum)                  |
| Dedicated storage nodes | No                    | Recommended                     |
| Base overhead           | ~1-2GB RAM            | ~4-6GB RAM                      |
| Per-volume overhead     | Engine + replicas     | Near zero                       |
| Practical max volumes   | ~500                  | 10,000+                         |
| IOPS                    | Good (iSCSI)          | Better (kernel RBD)             |
| Failure debugging       | Simple, readable logs | Complex (PG states, CRUSH maps) |
| k3s compatibility       | First-class           | Works with tuning               |

### Other Storage Options (Evaluated)

| Solution                   | Verdict                                                                          |
| -------------------------- | -------------------------------------------------------------------------------- |
| **OpenEBS Mayastor**       | Best NVMe I/O performance, but younger project with fewer production war stories |
| **DRBD/LINSTOR (Piraeus)** | Kernel-native, extremely mature, but commercial licensing for some features      |
| **Portworx**               | Enterprise best-in-class, but expensive commercial license                       |
| **Robin.io**               | App-aware snapshots, but commercial with less community support                  |

---

## Provider Abstractions

### VolumeProvider

Abstracts storage lifecycle so the deploy workflow never knows if it's Longhorn or Ceph underneath. The provider is resolved from the cluster's `storage_provider` column.

```go
type VolumeProvider interface {
    // Lifecycle
    CreateVolume(ctx context.Context, opts CreateVolumeOpts) (*Volume, error)
    DeleteVolume(ctx context.Context, name string, namespace string) error
    ResizeVolume(ctx context.Context, name string, namespace string, newSize string) error

    // Observability
    GetVolumeStatus(ctx context.Context, name string, namespace string) (*VolumeStatus, error)

    // Backup
    CreateSnapshot(ctx context.Context, name string, namespace string) (*Snapshot, error)
    RestoreSnapshot(ctx context.Context, snapshotID string, targetPVC string) error
}

type CreateVolumeOpts struct {
    Name       string
    Namespace  string
    Size       string // "10Gi"
    AccessMode string // "ReadWriteOnce"
    // No storage class — the provider decides
}

type VolumeStatus struct {
    Phase    string // Bound, Pending, Lost
    Size     string
    Node     string // which node it landed on
    Healthy  bool   // all replicas healthy?
    Replicas int    // how many copies exist
}
```

Implementations:

```go
type LonghornProvider struct {
    kubeClient   kubernetes.Interface
    storageClass string // "longhorn"
}

type CephProvider struct {
    kubeClient   kubernetes.Interface
    storageClass string // "rook-ceph-block"
}
```

### BuildProvider

Abstracts build infrastructure so each cluster can have its own BuildKit setup or shared build cluster.

```go
type BuildProvider interface {
    Build(ctx context.Context, opts BuildOpts) (*BuildResult, error)
    GetBuildStatus(ctx context.Context, buildID string) (*BuildStatus, error)
}

type BuildOpts struct {
    RepoURL   string
    CommitSHA string
    BuildPack string // "railpack", "dockerfile", "static"
    ImageTag  string // target image tag
    Registry  string // which registry to push to
}
```

### IngressProvider

Abstracts domain assignment and ingress creation. Multi-cluster means different Cloudflare LB pools per cluster, potentially different base domains per region.

```go
type IngressProvider interface {
    CreateIngress(ctx context.Context, opts IngressOpts) (*IngressResult, error)
    DeleteIngress(ctx context.Context, name string, namespace string) error
}

type IngressResult struct {
    URL         string // "https://my-app.ml.ink"
    InternalURL string // "my-app.ns.svc.cluster.local"
}
```

### ResourceProvider

Already partially abstracted with Turso. Same pattern for future database providers.

```go
type ResourceProvider interface {
    Create(ctx context.Context, opts CreateResourceOpts) (*Resource, error)
    Delete(ctx context.Context, name string) error
    GetConnectionInfo(ctx context.Context, name string) (*ConnectionInfo, error)
}
// Turso now, Neon Postgres later, self-hosted Postgres later
```

---

## Volume Feature Design

### How Volumes Work in Containers

Volumes don't replace the container's root filesystem. They mount a persistent disk at a specific path. Only writes to the mount path persist.

```
/               ← root filesystem (from container image, EPHEMERAL)
├── app/        ← application code (ephemeral)
├── tmp/        ← temp files (ephemeral)
├── data/       ← VOLUME (persistent, survives restarts/reschedules)
│   ├── uploads/
│   ├── sqlite.db
│   └── cache/
└── ...
```

### MCP Interface

```
create_service(
  repo="my-app",
  name="my-app",
  volumes=[{mount_path: "/data", size: "10Gi"}]
)
```

Default mount path: `/data` (user can override).

### Kubernetes Implementation

Services with volumes use **StatefulSet** instead of Deployment. StatefulSets give stable pod identity and persistent volume binding — the pod always reconnects to the same PVC across restarts.

```
create_service with volumes
  │
  ├─ Create PersistentVolumeClaim
  │    name: {service-name}-vol-0
  │    storageClass: (resolved from VolumeProvider)
  │    size: 10Gi
  │
  ├─ Use StatefulSet instead of Deployment
  │    - Stable pod identity
  │    - Pod reconnects to same PVC across restarts
  │
  └─ Mount PVC at /data in container spec
```

### Schema

```sql
CREATE TABLE volumes (
    id         TEXT PRIMARY KEY,
    service_id TEXT NOT NULL REFERENCES services(id),
    cluster_id TEXT NOT NULL REFERENCES clusters(id),
    mount_path TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    node_name  TEXT,              -- which run node it landed on
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### Tool Response

Include volume info and durability semantics in the response so agents understand the constraints:

```json
{
  "volume": {
    "mount_path": "/data",
    "size": "10Gi",
    "node": "run-1",
    "durability": "replicated",
    "replicas": 2,
    "backup_schedule": "daily"
  }
}
```

### Quotas and Limits

Enforce from day 1:

| Limit                    | Default                                         |
| ------------------------ | ----------------------------------------------- |
| Max volume size          | 50Gi (configurable per plan)                    |
| Max volumes per service  | 1                                               |
| Max volumes per customer | 5                                               |
| IOPS                     | No artificial limit (Longhorn/Ceph handle this) |

---

## Multi-Cluster Architecture

### Topology

```
┌──────────────────────────────────────────────┐
│              Product API / MCP               │
│                                              │
│  ┌────────────────────────────────────────┐  │
│  │          Cluster Router                │  │
│  │  (picks cluster for new service)       │  │
│  └───────────┬────────────────────────────┘  │
│              │                               │
│  ┌───────────▼────────────────────────────┐  │
│  │       Temporal Workflows               │  │
│  │  (deploy, delete, redeploy)            │  │
│  └───────────┬────────────────────────────┘  │
└──────────────┼───────────────────────────────┘
               │ task queue per cluster
      ┌────────┼────────┐
      ▼        ▼        ▼
  ┌───────┐┌───────┐┌───────┐
  │eu-w-1 ││us-e-1 ││ap-s-1 │  Deployer Workers
  │       ││       ││       │
  │ ┌───┐ ││ ┌───┐ ││ ┌───┐ │
  │ │Vol│ ││ │Vol│ ││ │Vol│ │  VolumeProvider (per cluster)
  │ │LH │ ││ │Cph│ ││ │LH │ │  Can differ per cluster
  │ └───┘ ││ └───┘ ││ └───┘ │
  │ ┌───┐ ││ ┌───┐ ││ ┌───┐ │
  │ │Bld│ ││ │Bld│ ││ │Bld│ │  BuildProvider (per cluster)
  │ └───┘ ││ └───┘ ││ └───┘ │
  └───────┘└───────┘└───────┘
```

### Key Properties

- Product API and Temporal remain centralized (on Railway)
- Each cluster runs one deployer-worker (same binary, different env config)
- Each cluster can have different storage providers (Longhorn on some, Ceph on others)
- Each cluster has its own registry, build infrastructure, and ingress
- Adding a cluster is: provision servers → run Ansible → add row to `clusters` table → deploy deployer-worker

### Why This Works Without a Rewrite

The current architecture already supports this naturally:

1. Product API lives on Railway, separate from k8s
2. Deployer-worker talks to k8s via client-go
3. Temporal routes work to workers via task queues

To go multi-cluster:

1. Add `cluster_id` column to services table
2. Give deployer-worker a config for which cluster it manages
3. Run one deployer-worker per cluster
4. Route in product API based on region/capacity

---

## Operational Concerns

### Backup Strategy (Day 1 Requirement)

Replication protects against node failure. Backups protect against everything else (accidental deletion, corruption, cluster-wide failure).

| What                   | Where                        | How                                        |
| ---------------------- | ---------------------------- | ------------------------------------------ |
| Volume data            | S3/R2/Hetzner Object Storage | Longhorn S3 backup target, scheduled daily |
| MCP state (Postgres)   | Supabase                     | Provider-managed backups                   |
| User databases (Turso) | Turso                        | Provider-native backups                    |
| Gitea repos            | ops-1 NVMe RAID1             | Built-in redundancy + periodic tar to S3   |
| Registry images        | Rebuildable                  | Not backed up — rebuild from source        |

### Monitoring Longhorn

- Longhorn dashboard or Grafana dashboards for volume health
- Alert on degraded volumes (replica count < desired)
- Alert on node storage capacity (>80% used)
- Monitor rebuild times after node recovery

### Adding/Replacing Nodes

Node death with Longhorn:

1. Longhorn auto-serves from surviving replicas, pod reschedules
2. Buy replacement server from Hetzner
3. Run `ansible-playbook add-run-node.yml --limit run-N`
4. Longhorn auto-rebuilds replicas onto new node
5. Done

### Storage and Compute Separation

Currently run nodes handle both compute (customer containers) and storage (Longhorn replicas). This works at small scale but eventually a disk-heavy customer affects neighbors.

**Future:** Dedicated storage nodes with NVMe, run nodes focus on compute. This is a natural split when adding Ceph (OSD daemons want dedicated I/O). Don't do it now — do it when you shard to the second cluster.

---

## What NOT to Abstract

Keep these concrete. Abstracting them adds complexity with no realistic swap-out scenario:

| Component                   | Why keep concrete                                                                                                     |
| --------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| **Container runtime (k8s)** | You're on k8s, you're staying on k8s. Don't abstract kubectl/client-go.                                               |
| **Git provider**            | Already well abstracted with `host=github.com` vs `host=ml.ink`. Sufficient.                                          |
| **Temporal**                | This is your workflow engine, period. No reason to swap.                                                              |
| **MCP tool interface**      | This is the agent contract. Keep it stable and simple. Users never see clusters, storage providers, or build systems. |

---

## Railway Learnings

Research into Railway's actual architecture revealed important lessons:

### What Railway Actually Built

- Railway spent all of 2022 trying to build on Kubernetes. It failed, nearly killed the company, and they deleted all the k8s code.
- They built a **custom container orchestrator** (no Kubernetes) with custom bare-metal scheduling.
- They use **Temporal** for deployment workflows (same as us).
- They built **MetalCP**, an internal control plane using Temporal + Redfish BMC APIs for bare metal provisioning.
- They have a **custom eBPF/Wireguard IPv6 mesh network** with automatic load balancing.
- Their network uses a **CLOS topology** with redundant switches and routing protocols.
- They run their **own data centers** in US, Europe, and Southeast Asia.
- They serve 1.7M+ users with hundreds of thousands of deployed applications.

### Key Takeaways

1. **Don't prematurely optimize infrastructure.** Railway shipped on GCP for years before building data centers. They tried k8s too early and it nearly killed them.

2. **Railway left k8s for product reasons, not scale reasons.** They wanted sub-second deploys and extreme control over scheduling and networking. Most companies (Shopify, Spotify, CERN) run massive deployments on k8s successfully.

3. **Volumes on Railway are local NVMe with platform-managed migration and backups.** Not network-attached storage, not Longhorn/Ceph. They built custom volume management because at their scale the engineering cost pays for itself.

4. **Temporal is the right orchestration choice.** Railway uses it for everything — deploys, volume lifecycle, bare metal provisioning. We use it the same way.

5. **The multi-cluster path is natural.** Our product API is already separated from k8s deployer infrastructure. Multi-cluster is a config change, not a rewrite.

---

## Implementation Priority

What to build now vs later:

| Item                                          | Priority  | Effort  | Why now                                            |
| --------------------------------------------- | --------- | ------- | -------------------------------------------------- |
| `clusters` table + task queue routing         | **Now**   | Small   | Makes multi-cluster a config change, not a rewrite |
| `cluster_id` on services/volumes              | **Now**   | Trivial | Know where everything lives                        |
| Deployer-worker reads cluster config from env | **Now**   | Trivial | Same binary, different clusters                    |
| `VolumeProvider` interface                    | **Now**   | Small   | Swap Longhorn → Ceph without touching workflows    |
| Volume backups to S3                          | **Now**   | Medium  | Real disaster recovery, non-negotiable             |
| Volume size limits and quotas                 | **Now**   | Small   | Prevent resource exhaustion from day 1             |
| `BuildProvider` interface                     | **Soon**  | Small   | Per-cluster build infra later                      |
| `IngressProvider` interface                   | **Soon**  | Small   | Per-cluster domain routing                         |
| Cluster placement logic                       | **Later** | Medium  | Only needed with 2+ clusters                       |
| Dedicated storage nodes                       | **Later** | Medium  | Only needed when I/O isolation matters             |
| Ceph deployment                               | **Later** | Large   | Only on new clusters when Longhorn limits hit      |
