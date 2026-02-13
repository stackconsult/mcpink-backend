# Deploy MCP - Architecture Plan

## scope
This document defines the high-level runtime architecture for the k3s platform that builds and runs customer workloads.

The product (MCP API, Temporal Cloud, Postgres, auth) stays on Railway.

This plan intentionally avoids low-level manifests and command-level procedures. For implementation details, use:
- `infra/ansible` for host provisioning, network/bootstrap, hardening, and node lifecycle
- `infra/eu-central-1/k8s` for cluster workloads, routing objects, runtime controls, and observability resources

## responsibility split
Railway is the product plane and system-of-record boundary. It owns API contracts, auth, metadata, and workflow orchestration triggers.

k3s is the execution plane. It runs the worker/build/deploy path, serves tenant runtime traffic, and hosts platform observability components.

External contracts remain stable:
- MCP endpoint contract: `mcp.ml.ink/mcp`
- Webhook ingress host: `api.ml.ink` (unchanged)

## topology/pools
The cluster is segmented into four operational pools:
- Control pool: cluster control-plane services and workflow worker coordination
- Ops pool: shared platform services such as image storage and observability
- Build pool: isolated image build capacity
- Run pool: customer-facing ingress and tenant workloads

Placement policy keeps build and ops concerns isolated from tenant runtime execution, while run capacity scales horizontally for customer traffic.

## traffic flow
Cloudflare Load Balancer is the public ingress model.

Public request path is:
- client traffic enters Cloudflare
- Cloudflare routes to healthy run-pool origins
- run-pool ingress routes by host/path to in-cluster services
- service traffic reaches tenant workloads on the cluster network

Control path is:
- Railway APIs trigger orchestration and deployment workflows
- worker/build components produce images and deploy workloads
- logs and status telemetry flow back to the product plane for user-visible state

Webhook traffic continues to enter at `api.ml.ink`, and MCP clients continue to target `mcp.ml.ink/mcp`.

## security posture
Security posture is defense-in-depth:
- workload isolation at runtime boundary for tenant code
- namespace and network segmentation between tenant and platform concerns
- restricted east-west and outbound paths for tenant workloads
- private-path handling for internal build and image distribution
- secret handling kept out of workflow history and minimized to runtime use
- least-privilege service identities for cluster and control interactions

## operations model
Operational ownership is split between declarative host automation and declarative cluster resources:
- `infra/ansible` is the source for node bootstrap, OS/network policy, and fleet changes
- `infra/eu-central-1/k8s` is the source for in-cluster services, routing, scheduling, and policies

Steady-state operations center on:
- pool capacity management, especially run-pool scaling
- ingress health and routing continuity through Cloudflare
- build throughput and image lifecycle hygiene
- observability-driven incident response and rollback decisions

## DR and known gaps
Current disaster recovery posture prioritizes fast rebuild from infrastructure definitions and reproducible cluster state, not full multi-region high availability.

Known gaps and constraints:
- control-plane and certain platform services remain concentrated, creating failure-domain coupling
- recovery depends on infrastructure re-provisioning plus config replay
- some operational steps still require explicit operator action during failover events
- multi-region and deeper redundancy are future hardening items

These limits are acceptable for the current phase, with planned evolution toward stronger control-plane and platform-service redundancy.
