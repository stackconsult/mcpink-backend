# k8s-worker

Temporal worker that handles service builds and deployments on k3s. Task queue is read from the `clusters` table (e.g. `deployer-eu-central-1`).

## Prerequisites

- Go 1.24+
- `kubectl` configured with access to the k3s cluster
- Postgres running (same DB as the API server)
- Temporal server running
- `git` installed

## Running Locally

### 1. Port-forward cluster services

```bash
# BuildKit (required for builds)
kubectl port-forward -n dp-system svc/buildkitd 1234:1234 &

# Registry (required for image push)
kubectl port-forward -n dp-system svc/registry 5000:5000 &

# Loki (optional, for build/run logs)
kubectl port-forward -n dp-system svc/loki 3100:3100 &
```

### 2. Set environment variables

```bash
export K8SWORKER_BUILDKITHOST="tcp://localhost:1234"
export K8SWORKER_REGISTRYHOST="http://localhost:5000"
export K8SWORKER_REGISTRYADDRESS="registry.internal:5000"
export K8SWORKER_LOKIPUSHURL="http://localhost:3100/loki/api/v1/push"
export K8SWORKER_LOKIQUERYURL="http://localhost:3100/loki/api/v1/query_range"
```

DB and Temporal use defaults from `application.yaml`. Override if needed:

```bash
export DB_URL="postgres://deploy:deploy@localhost:5432/deploymcp?sslmode=disable"
export TEMPORAL_ADDRESS="localhost:7233"
```

### 3. Run

```bash
# From go-backend/
make run-k8s-worker
```

## How it connects to k8s

The worker tries in-cluster config first (when deployed as a pod), then falls back to `~/.kube/config` (for local dev). Your local kubeconfig must have permissions to create namespaces, deployments, services, ingresses, secrets, network policies, and resource quotas.

## Workflows

| Workflow | Description |
|----------|-------------|
| `CreateServiceWorkflow` | Clone → Build → Deploy → WaitForRollout |
| `RedeployServiceWorkflow` | Same as Create (new image, rolling update) |
| `DeleteServiceWorkflow` | Delete Ingress, Service, Deployment, Secret |
| `BuildServiceWorkflow` | Child workflow: Clone → Resolve → Build (railpack/dockerfile/static) |

## Triggering a test workflow

Use Temporal UI or `tctl`:

```bash
temporal workflow start \
  --task-queue deployer-eu-central-1 \
  --type CreateServiceWorkflow \
  --input '{"ServiceID":"<app-id>","Repo":"owner/repo","Branch":"main","GitProvider":"github","InstallationID":12345,"CommitSHA":"","AppsDomain":"ml.ink"}'
```
