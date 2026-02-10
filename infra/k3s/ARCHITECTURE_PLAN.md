# Deploy MCP — v0 Infrastructure Plan

> The k3s cluster that builds and runs customer applications.
> The product (MCP API, Temporal Cloud, Postgres, auth) stays on Railway.

---

## 0. Feedback Fixes Applied

| Issue                                                      | Fix                                                                                                     |
| ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| Taints deadlock kube-system pods                           | Remove taint from ctrl and run. Only taint ops and build. Customer isolation via nodeSelector + gVisor. |
| Wildcard TLS secret can't cross namespaces                 | Traefik TLSStore default cert. Customer Ingresses don't reference any secret.                           |
| ops/loki DNS points to ops-1 but Traefik runs on run nodes | All public domains point to run nodes. Traefik routes to ops-1 services over cluster network.           |
| Traefik CRDs not installed                                 | Install Traefik via Helm. Helm manages CRDs.                                                            |
| No egress controls on customer pods                        | Default-deny egress. Allowlist DNS + public internet. Block private net, k8s API, registry, BuildKit.   |
| Registry hostPort exposed to internet                      | Host firewall: port 5000 only from 10.0.1.0/24.                                                         |
| Secrets in Temporal workflow inputs                        | Pass identifiers only. Worker mints tokens inside activities from k8s Secrets.                          |
| Hostname collisions across tenants                         | Service name is globally unique (checked in Postgres). No tenant/project in domain.                     |
| registry.internal doesn't resolve in pods                  | In-cluster pods use IP (10.0.1.10:5000). Nodes use /etc/hosts for kubelet image pulls.                  |

---

## 1. Responsibility Split

```
┌─────────────────────────────────────────────────────────────────┐
│  RAILWAY — Your Product (not part of this plan)                 │
│                                                                 │
│  MCP API (Go)                                                   │
│    ├─ Receives MCP tool calls from agents                       │
│    ├─ WRITES: starts Temporal workflows (create, redeploy,      │
│    │   delete)                                                  │
│    ├─ READS: queries own Postgres for service/resource metadata  │
│    ├─ READS: queries Loki for build + runtime logs              │
│    ├─ READS: queries k8s API for live pod status                │
│    └─ READS: queries registry API for image info                │
│                                                                 │
│  Postgres — metadata (projects, services, users, resources)     │
│  Temporal Cloud — workflow orchestration                        │
│  Auth — GitHub OAuth, API keys                                  │
└────────────────────┬────────────────────────────────────────────┘
                     │
        ┌────────────┼─────────────────────┐
        │            │                     │
        ▼            ▼                     ▼
   Temporal Cloud   Loki                  k8s API
   (start workflows) (log queries)        (pod status)
   gRPC outbound    HTTPS                 HTTPS
        │            loki.breacher.org     k3s-1 public IP
        │
        ▼
┌───────────────────────────────────────────────────────────────┐
│  K3S CLUSTER — This Plan                                      │
│                                                               │
│  Temporal Worker (on k3s-1)                                   │
│    ├─ Picks up workflows from Temporal Cloud                  │
│    ├─ Activity: clone repo                                    │
│    ├─ Activity: build image via BuildKit                      │
│    ├─ Activity: kubectl apply (Deployment/Service/Ingress)    │
│    ├─ Activity: wait for rollout                              │
│    └─ Returns: {status, url, commit_sha, error}               │
│                                                               │
│  BuildKit (on build-1) — builds images                        │
│  Registry (on ops-1) — stores images                          │
│  Traefik (on run nodes) — routes all public traffic           │
│  gVisor (on run nodes) — sandboxes customer code              │
│  Loki (on ops-1) — log storage                                │
│  Prometheus + Grafana (on ops-1) — metrics + dashboards       │
│  Registry GC CronJob — keeps last 2 images per service        │
│  Customer containers (on run nodes)                           │
└───────────────────────────────────────────────────────────────┘
```

### What talks to what

| From                | To                 | How                   | Purpose                           |
| ------------------- | ------------------ | --------------------- | --------------------------------- |
| MCP API (Railway)   | Temporal Cloud     | gRPC                  | Start workflows, read results     |
| MCP API (Railway)   | loki.breacher.org  | HTTPS + bearer token  | Logs for `get_service`            |
| MCP API (Railway)   | k3s-1:6443         | HTTPS + SA token      | Pod status for `list_services`    |
| Temporal Worker     | Temporal Cloud     | gRPC outbound         | Poll tasks, report results        |
| Temporal Worker     | BuildKit (build-1) | TCP 1234, cluster DNS | Drive builds                      |
| Temporal Worker     | Registry (ops-1)   | HTTP, 10.0.1.10:5000  | Push images (IP, not DNS)         |
| Temporal Worker     | k8s API            | In-cluster SA         | kubectl apply, rollout status     |
| Temporal Worker     | Loki (ops-1)       | HTTP, cluster DNS     | Push build logs                   |
| Run nodes (kubelet) | Registry (ops-1)   | HTTP, 10.0.1.10:5000  | Pull images (via registries.yaml) |

---

## 2. Node Architecture

```
┌──────────────────────────────────────────────────────────────┐
│  k3s-1 — K3s Server (Hetzner Cloud CX32)                    │
│                                                              │
│  k3s server process (etcd, API server, scheduler)            │
│  Temporal Worker                                             │
│  cert-manager controller                                     │
│  CoreDNS, metrics-server (system pods land here)             │
│                                                              │
│  Labels: pool=ctrl                                           │
│  Taint: NONE (system pods need somewhere to run)             │
└──────────────────────────────────────────────────────────────┘
         │ private network (10.0.1.0/24)
         │
┌────────┴─────────────────────────────────────────────────────┐
│  ops-1 — Storage & Observability (Hetzner Dedicated)         │
│                                                              │
│  Docker Registry (NVMe-backed)                               │
│  Registry GC CronJob                                         │
│  Loki, Prometheus, Grafana                                   │
│                                                              │
│  Labels: pool=ops                                            │
│  Taint: pool=ops:NoSchedule                                  │
└──────────────────────────────────────────────────────────────┘
         │
┌────────┴─────────────────────────────────────────────────────┐
│  build-1 — Builder (Hetzner Cloud CCX, dedicated CPU)        │
│                                                              │
│  BuildKit daemon (persistent local cache + registry cache)   │
│                                                              │
│  Labels: pool=build                                          │
│  Taint: pool=build:NoSchedule                                │
└──────────────────────────────────────────────────────────────┘
         │
┌────────┴─────────────────────────────────────────────────────┐
│  run-1, run-2, ... — Runners (Hetzner Dedicated)             │
│                                                              │
│  Traefik (DaemonSet, hostNetwork)                            │
│  gVisor (runsc RuntimeClass)                                 │
│  Customer containers                                         │
│                                                              │
│  Labels: pool=run                                            │
│  Taint: NONE (customer pods + Traefik + system DaemonSets)   │
└──────────────────────────────────────────────────────────────┘
```

### Taint Strategy

Only ops and build are tainted — they run dedicated infrastructure that shouldn't be disturbed by random scheduling. ctrl and run are untainted so kube-system pods (CoreDNS, metrics-server, etc.) can land on them without special tolerations.

Customer pods only land on run nodes because their Deployments specify `runtimeClassName: gvisor`, and the gVisor RuntimeClass has `scheduling.nodeSelector: pool=run`. No taint needed.

### Sizing

| Node    | Type                 | Specs                       | Why                                            |
| ------- | -------------------- | --------------------------- | ---------------------------------------------- |
| k3s-1   | Cloud CX32           | 4 vCPU, 8GB, 80GB           | k3s server + Temporal worker. Headroom for HA. |
| ops-1   | Auction AX42+        | 6C+, 64GB, 2× NVMe          | Registry + Loki + Prometheus need disk.        |
| build-1 | Cloud CCX23 or CCX33 | 4-8 dedicated vCPU, 16-32GB | Builds are CPU-bound.                          |
| run-1   | Auction AX42         | 6C/12T, 64GB                | Customer workloads.                            |

### Current Infrastructure Status

| Node    | Current Name   | Status                                                    |
| ------- | -------------- | --------------------------------------------------------- |
| k3s-1   | `k3s-1`        | **Clean.** Ready to use.                                  |
| ops-1   | `muscle-ops-1` | Has Coolify installed. Wipe after k3s migration verified. |
| build-1 | `builder-1`    | Has Coolify installed. Wipe after k3s migration verified. |
| run-1   | `muscle-1`     | Has Coolify installed. Wipe after k3s migration verified. |

**Migration strategy:** Stand up k3s alongside Coolify. Once all MCP tools work, cut DNS, verify, then wipe Coolify. Both coexist since k3s uses different ports and the private network.

---

## 3. Networking

### Private Network (Hetzner vSwitch)

```
10.0.1.1       k3s-1      (Cloud, auto-assigned)
10.0.1.2       build-1    (Cloud, auto-assigned)
10.0.1.10      ops-1      (Dedicated, static Netplan)
10.0.1.100     run-1      (Dedicated, static Netplan)
10.0.1.101     run-2      (future)
```

Setup:

1. Robot → create vSwitch (VLAN 4001), attach dedicated servers
2. Cloud Console → create Network `deploy-vpc` (`10.0.0.0/16`), subnet `10.0.1.0/24` type "vSwitch" linked to Robot vSwitch, attach Cloud servers
3. Dedicated servers → Netplan VLAN config (Ansible `vswitch` role)

### DNS

```
*.breacher.org       A    <run-1-ip>       # add A record per run node
ops.breacher.org     A    <run-1-ip>       # Grafana — Traefik routes to ops-1 over cluster net
loki.breacher.org    A    <run-1-ip>       # Loki — same
```

All public traffic enters through run nodes. Traefik routes to backend services wherever they run. ops-1 never needs to accept public web traffic.

### TLS

Wildcard cert in `dp-system`. Traefik uses it as default cert via TLSStore — customer Ingresses don't reference any secret.

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: wildcard-apps
  namespace: dp-system
spec:
  secretName: wildcard-apps-tls
  issuerRef:
    name: letsencrypt-prod
    kind: ClusterIssuer
  dnsNames:
    - "*.breacher.org"
---
apiVersion: traefik.io/v1alpha1
kind: TLSStore
metadata:
  name: default
  namespace: dp-system
spec:
  defaultCertificate:
    secretName: wildcard-apps-tls
```

---

## 4. Domain Naming

### How it works

The `name` field in `create_service` IS the subdomain. Globally unique, checked in Postgres.

```
create_service(name="my-app", ...)   →  https://my-app.breacher.org
create_service(name="cool-api", ...) →  https://cool-api.breacher.org
```

- No `service-project-tenant` formula. The name is the domain.
- Uniqueness enforced in Postgres before deployment starts.
- If taken: return error, agent picks a different name.
- Project/tenant are internal metadata, not in the URL.

### Future: custom domains

Not in v0. But the architecture supports it:

1. User says "use mycoolapp.com"
2. Future `add_domain` tool
3. User adds CNAME: `mycoolapp.com → my-app.breacher.org`
4. cert-manager issues cert via HTTP-01
5. Additional Ingress rule routes both domains to same service

### Routing

```
*.breacher.org → DNS round-robin across all run node IPs
               → Traefik (hostNetwork)
               → k8s Service
               → Pod (any run node)
```

Pod moves → k8s updates endpoints → Traefik discovers change. Domain keeps working.

---

## 5. Build Pipeline

### Flow

```
MCP API (Railway) → Temporal Cloud → Worker (k3s-1) picks up task
  │
  ├─ Activity: CloneRepo
  │    Receives: repo name, git host, installation ID (NOT tokens)
  │    Worker mints GitHub App token from private key in k8s Secret
  │    For Gitea: fetches token from k8s Secret
  │    git clone over HTTPS
  │
  ├─ Activity: BuildAndPush
  │    BuildKit client at tcp://buildkitd.dp-system:1234
  │    Railpack generates build plan (or use repo Dockerfile)
  │    buildctl build:
  │      --import-cache type=registry,ref=10.0.1.10:5000/cache/<service>:latest
  │      --export-cache type=registry,ref=10.0.1.10:5000/cache/<service>:latest,mode=max
  │      --output type=image,name=registry.internal:5000/<namespace>/<service>:<sha>,push=true
  │    Heartbeats: small progress string (not full logs)
  │    Build logs pushed to Loki (HTTP POST to loki.dp-system:3100)
  │
  ├─ Activity: Deploy
  │    Ensure namespace exists (idempotent)
  │    Fetch env vars from MCP API (authenticated call) — never in workflow input
  │    Apply: Secret, Deployment, Service, Ingress
  │    kubectl rollout status (timeout 120s)
  │
  └─ Return: {status: "running", url: "my-app.breacher.org", commit_sha: "abc123"}
```

**No secrets in Temporal history.** Workflow input: service ID, repo name, installation ID, commit SHA. Worker mints/fetches secrets inside activities.

### Cache

Two layers:

1. **Local cache on build-1** — fastest. `--oci-worker-gc-keepstorage=20000` (20GB). Lost on node replacement.
2. **Registry cache on ops-1** — persistent, shared. `mode=max` caches every intermediate layer.

### Speed Targets

| Scenario                     | Target |
| ---------------------------- | ------ |
| Cold build                   | < 120s |
| Cached rebuild (code change) | < 15s  |
| Full cache hit               | < 5s   |
| Image pull (private net)     | < 3s   |

### Image Cleanup

CronJob on ops-1, nightly. Keeps last 2 tags per service. Runs `registry garbage-collect --delete-untagged`. No Temporal.

### Build Packs

| Value                | Behavior                                             |
| -------------------- | ---------------------------------------------------- |
| `railpack` (default) | Detect language, generate build plan, drive BuildKit |
| `dockerfile`         | Use repo's Dockerfile                                |
| `static`             | Built-in Caddy Dockerfile                            |

---

## 6. Temporal Worker

Go binary in your repo. Registers activities with Temporal Cloud. Deployed on k3s-1.

### Does

| Activity       | Needs                                                       |
| -------------- | ----------------------------------------------------------- |
| CloneRepo      | Git token (minted from k8s Secret, NOT from workflow input) |
| BuildAndPush   | BuildKit client (cluster DNS), registry push (IP)           |
| Deploy         | k8s API (in-cluster SA), env vars (fetched from MCP API)    |
| WaitForRollout | k8s API                                                     |
| DeleteService  | k8s API                                                     |

### Does NOT

- Serve HTTP
- Query logs
- Clean up images
- Handle auth
- Anything MCP-read related

### Manifest

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: temporal-worker
  namespace: dp-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: temporal-worker
  template:
    metadata:
      labels:
        app: temporal-worker
    spec:
      nodeSelector:
        pool: ctrl
      serviceAccountName: temporal-worker
      containers:
        - name: worker
          image: registry.internal:5000/dp-system/temporal-worker:latest
          env:
            - name: TEMPORAL_ADDRESS
              value: "<namespace>.tmprl.cloud:7233"
            - name: TEMPORAL_NAMESPACE
              value: "<namespace>"
            - name: TEMPORAL_TLS_CERT
              valueFrom:
                secretKeyRef:
                  name: temporal-creds
                  key: tls-cert
            - name: TEMPORAL_TLS_KEY
              valueFrom:
                secretKeyRef:
                  name: temporal-creds
                  key: tls-key
            - name: BUILDKIT_HOST
              value: "tcp://buildkitd.dp-system:1234"
            - name: REGISTRY_HOST
              value: "http://10.0.1.10:5000"
            - name: LOKI_PUSH_URL
              value: "http://loki.dp-system:3100/loki/api/v1/push"
            - name: GITHUB_APP_PRIVATE_KEY
              valueFrom:
                secretKeyRef:
                  name: github-app
                  key: private-key
          resources:
            requests:
              cpu: "200m"
              memory: 256Mi
            limits:
              cpu: "1"
              memory: 512Mi
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: temporal-worker
  namespace: dp-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: temporal-worker
rules:
  - apiGroups: [""]
    resources: [namespaces, services, secrets, resourcequotas]
    verbs: [get, list, create, update, patch, delete]
  - apiGroups: [apps]
    resources: [deployments]
    verbs: [get, list, watch, create, update, patch, delete]
  - apiGroups: [networking.k8s.io]
    resources: [ingresses, networkpolicies]
    verbs: [get, list, create, update, patch, delete]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: temporal-worker
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: temporal-worker
subjects:
  - kind: ServiceAccount
    name: temporal-worker
    namespace: dp-system
```

### Building the worker image

```bash
docker build -f Dockerfile.worker -t registry.internal:5000/dp-system/temporal-worker:latest .
docker push registry.internal:5000/dp-system/temporal-worker:latest
kubectl rollout restart deployment/temporal-worker -n dp-system
```

---

## 7. K8s Manifests

All in `infra/k8s/`. Applied by Ansible. Living documentation.

### dp-system.yml

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: dp-system
  labels:
    app.kubernetes.io/part-of: deploy-mcp
```

### registry.yml

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: registry
  namespace: dp-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: registry
  template:
    metadata:
      labels:
        app: registry
    spec:
      nodeSelector:
        pool: ops
      tolerations:
        - key: pool
          value: ops
          effect: NoSchedule
      containers:
        - name: registry
          image: registry:2
          ports:
            - containerPort: 5000
              hostPort: 5000
          env:
            - name: REGISTRY_STORAGE_DELETE_ENABLED
              value: "true"
          volumeMounts:
            - name: data
              mountPath: /var/lib/registry
          resources:
            requests:
              cpu: "200m"
              memory: 256Mi
            limits:
              cpu: "1"
              memory: 1Gi
      volumes:
        - name: data
          hostPath:
            path: /mnt/registry
            type: DirectoryOrCreate
---
apiVersion: v1
kind: Service
metadata:
  name: registry
  namespace: dp-system
spec:
  selector:
    app: registry
  ports:
    - port: 5000
```

**Firewall (Ansible `firewall` role on ops-1):** Block port 5000 from public, allow only private network:

```bash
iptables -A INPUT -p tcp --dport 5000 -s 10.0.1.0/24 -j ACCEPT
iptables -A INPUT -p tcp --dport 5000 -j DROP
```

### registry-gc.yml

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: registry-gc
  namespace: dp-system
spec:
  schedule: "0 4 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          nodeSelector:
            pool: ops
          tolerations:
            - key: pool
              value: ops
              effect: NoSchedule
          containers:
            - name: gc
              image: registry:2
              command:
                - /bin/sh
                - -c
                - /bin/registry garbage-collect /etc/docker/registry/config.yml --delete-untagged
              volumeMounts:
                - name: data
                  mountPath: /var/lib/registry
          volumes:
            - name: data
              hostPath:
                path: /mnt/registry
          restartPolicy: OnFailure
```

### buildkit.yml

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: buildkitd
  namespace: dp-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: buildkitd
  template:
    metadata:
      labels:
        app: buildkitd
    spec:
      nodeSelector:
        pool: build
      tolerations:
        - key: pool
          value: build
          effect: NoSchedule
      containers:
        - name: buildkitd
          image: moby/buildkit:latest
          args:
            - "--addr"
            - "tcp://0.0.0.0:1234"
            - "--oci-worker-gc"
            - "--oci-worker-gc-keepstorage=20000"
          ports:
            - containerPort: 1234
          securityContext:
            privileged: true
          volumeMounts:
            - name: state
              mountPath: /var/lib/buildkit
          resources:
            requests:
              cpu: "4"
              memory: 16Gi
            limits:
              cpu: "8"
              memory: 28Gi
      volumes:
        - name: state
          hostPath:
            path: /mnt/buildkit
            type: DirectoryOrCreate
---
apiVersion: v1
kind: Service
metadata:
  name: buildkitd
  namespace: dp-system
spec:
  selector:
    app: buildkitd
  ports:
    - port: 1234
```

### traefik (Helm)

Install via Helm to get CRDs and IngressClass:

```bash
helm repo add traefik https://traefik.github.io/charts
helm install traefik traefik/traefik -n dp-system \
  --set deployment.kind=DaemonSet \
  --set nodeSelector.pool=run \
  --set service.type=ClusterIP \
  --set hostNetwork=true \
  --set ingressClass.enabled=true \
  --set ingressClass.isDefaultClass=true \
  --set ports.web.hostPort=80 \
  --set ports.websecure.hostPort=443 \
  --set ports.web.redirectTo.port=websecure \
  --set providers.kubernetesIngress.enabled=true \
  --set providers.kubernetesCRD.enabled=true \
  --set logs.general.level=WARN \
  --set resources.requests.cpu=100m \
  --set resources.requests.memory=128Mi \
  --set resources.limits.cpu=500m \
  --set resources.limits.memory=256Mi
```

### cert-manager

```bash
helm repo add jetstack https://charts.jetstack.io
helm install cert-manager jetstack/cert-manager -n cert-manager --create-namespace \
  --set crds.enabled=true
```

```yaml
# infra/k8s/cert-manager-issuer.yml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: ops@ml.ink
    privateKeySecretRef:
      name: letsencrypt-prod-key
    solvers:
      - dns01:
          cloudflare:
            email: ops@ml.ink
            apiTokenSecretRef:
              name: cloudflare-api-token
              key: api-token
        selector:
          dnsZones:
            - "breacher.org"
```

### gvisor-runtimeclass.yml

```yaml
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: gvisor
handler: runsc
scheduling:
  nodeSelector:
    pool: run
```

No tolerations needed — run nodes are untainted.

### Loki Ingress (for Railway access)

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: loki
  namespace: dp-system
  annotations:
    traefik.ingress.kubernetes.io/router.middlewares: dp-system-loki-auth@kubernetescrd
spec:
  rules:
    - host: loki.breacher.org
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: loki
                port:
                  number: 3100
---
# Bearer token auth for Loki
apiVersion: traefik.io/v1alpha1
kind: Middleware
metadata:
  name: loki-auth
  namespace: dp-system
spec:
  basicAuth:
    secret: loki-auth-secret
---
apiVersion: v1
kind: Secret
metadata:
  name: loki-auth-secret
  namespace: dp-system
type: Opaque
stringData:
  users: "deploy:<bcrypt-hashed-password>"
```

MCP API on Railway queries: `https://deploy:<password>@loki.breacher.org/loki/api/v1/query_range?query=...`

---

## 8. Customer Namespace Template

Generated by Temporal worker on first `create_service` for a project:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: dp-{{ tenant }}-{{ project }}
  labels:
    dp.ml.ink/tenant: "{{ tenant }}"
    dp.ml.ink/project: "{{ project }}"
---
# Ingress: only from same namespace + Traefik
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: ingress-isolation
  namespace: dp-{{ tenant }}-{{ project }}
spec:
  podSelector: {}
  policyTypes: [Ingress]
  ingress:
    - from:
        - podSelector: {}
    - from:
        - namespaceSelector:
            matchLabels:
              app.kubernetes.io/part-of: deploy-mcp
          podSelector:
            matchLabels:
              app.kubernetes.io/name: traefik
---
# Egress: allow DNS + public internet, block private infra
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: egress-isolation
  namespace: dp-{{ tenant }}-{{ project }}
spec:
  podSelector: {}
  policyTypes: [Egress]
  egress:
    # Allow DNS
    - ports:
        - protocol: UDP
          port: 53
        - protocol: TCP
          port: 53
    # Allow public internet, block private ranges and k8s API
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
            except:
              - 10.0.0.0/8 # private network (registry, buildkit, nodes)
              - 172.16.0.0/12 # pod/service CIDRs
              - 192.168.0.0/16
              - 169.254.169.254/32 # cloud metadata
---
apiVersion: v1
kind: ResourceQuota
metadata:
  name: quota
  namespace: dp-{{ tenant }}-{{ project }}
spec:
  hard:
    pods: "20"
    requests.cpu: "4"
    requests.memory: 4Gi
    limits.cpu: "8"
    limits.memory: 8Gi
```

---

## 9. Customer Service Template

Generated per `create_service`:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: {{ service }}-env
  namespace: dp-{{ tenant }}-{{ project }}
type: Opaque
stringData:
  PORT: "{{ port }}"
  # + user env vars + resource-injected vars
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ service }}
  namespace: dp-{{ tenant }}-{{ project }}
  labels:
    app: {{ service }}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: {{ service }}
  template:
    metadata:
      labels:
        app: {{ service }}
    spec:
      runtimeClassName: gvisor
      automountServiceAccountToken: false
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
      containers:
        - name: app
          image: registry.internal:5000/dp-{{ tenant }}-{{ project }}/{{ service }}:{{ sha }}
          ports:
            - containerPort: {{ port }}
          envFrom:
            - secretRef:
                name: {{ service }}-env
          resources:
            requests:
              cpu: "{{ cpu }}"
              memory: "{{ memory }}"
            limits:
              cpu: "{{ cpu }}"
              memory: "{{ memory }}"
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: false
            capabilities:
              drop: ["ALL"]
---
apiVersion: v1
kind: Service
metadata:
  name: {{ service }}
  namespace: dp-{{ tenant }}-{{ project }}
spec:
  selector:
    app: {{ service }}
  ports:
    - port: {{ port }}
      targetPort: {{ port }}
---
# Ingress — no tls block needed, Traefik serves wildcard cert via TLSStore
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ service }}
  namespace: dp-{{ tenant }}-{{ project }}
spec:
  rules:
    - host: {{ name }}.breacher.org
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: {{ service }}
                port:
                  number: {{ port }}
```

Note: `{{ name }}` is the globally unique service name (the domain), `{{ service }}` is the k8s resource name (same value). The Ingress host uses `name` — the user-chosen subdomain.

Services in the same project reach each other by k8s DNS: `http://api:3000` from the `web` service.

---

## 10. Observability

All on `pool=ops` except DaemonSets.

```bash
# Loki
helm install loki grafana/loki -n dp-system \
  --set loki.auth_enabled=false \
  --set loki.storage.type=filesystem \
  --set singleBinary.replicas=1 \
  --set singleBinary.nodeSelector.pool=ops \
  --set singleBinary.tolerations[0].key=pool,tolerations[0].value=ops,tolerations[0].effect=NoSchedule \
  --set singleBinary.persistence.enabled=true \
  --set singleBinary.persistence.size=50Gi

# Promtail (all nodes — no tolerations needed, ctrl/run untainted)
helm install promtail grafana/promtail -n dp-system \
  --set config.clients[0].url=http://loki.dp-system:3100/loki/api/v1/push \
  --set tolerations[0].key=pool,tolerations[0].operator=Exists

# Prometheus + Node Exporter
helm install prometheus prometheus-community/kube-prometheus-stack -n dp-system \
  --set prometheus.prometheusSpec.nodeSelector.pool=ops \
  --set prometheus.prometheusSpec.tolerations[0].key=pool,tolerations[0].value=ops,tolerations[0].effect=NoSchedule \
  --set grafana.enabled=false \
  --set prometheus.prometheusSpec.retention=30d \
  --set nodeExporter.tolerations[0].key=pool,nodeExporter.tolerations[0].operator=Exists

# Grafana
helm install grafana grafana/grafana -n dp-system \
  --set nodeSelector.pool=ops \
  --set tolerations[0].key=pool,tolerations[0].value=ops,tolerations[0].effect=NoSchedule \
  --set persistence.enabled=true \
  --set persistence.size=10Gi
```

Grafana Ingress (routed through run nodes like everything else):

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: grafana
  namespace: dp-system
spec:
  rules:
    - host: ops.breacher.org
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: grafana
                port:
                  number: 80
```

### Dashboards

| Dashboard      | Shows                              |
| -------------- | ---------------------------------- |
| Cluster Health | All nodes: CPU, mem, disk, network |
| Build Pipeline | Duration, cache hit rate, failures |
| Tenant Usage   | Per-namespace CPU, mem, pod count  |
| Registry       | Disk usage, image count            |

### Log Queries from MCP API

```
# Runtime logs
GET https://deploy:<pw>@loki.breacher.org/loki/api/v1/query_range
  ?query={namespace="dp-max-catapp",app="api"}&limit=50&direction=backward

# Build logs
GET https://deploy:<pw>@loki.breacher.org/loki/api/v1/query_range
  ?query={job="build",tenant="max",project="catapp",service="api"}&limit=50&direction=backward
```

---

## 11. k3s Cluster Setup

### Server (k3s-1)

```bash
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="server \
  --cluster-init \
  --disable traefik \
  --disable servicelb \
  --tls-san <k3s-1-public-ip> \
  --tls-san 10.0.1.1 \
  --node-label pool=ctrl \
  --flannel-iface eth1" sh -
```

No taint on ctrl. System pods land here.

### Agents

```bash
# ops-1 and build-1 (tainted)
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="agent \
  --server https://10.0.1.1:6443 \
  --token <k3s-token> \
  --node-label pool=<ops|build> \
  --node-taint pool=<ops|build>:NoSchedule \
  --flannel-iface <private-iface>" sh -

# run nodes (NOT tainted)
curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="agent \
  --server https://10.0.1.1:6443 \
  --token <k3s-token> \
  --node-label pool=run \
  --flannel-iface <private-iface>" sh -
```

### Registry access (all nodes)

```yaml
# /etc/rancher/k3s/registries.yaml
mirrors:
  "registry.internal:5000":
    endpoint:
      - "http://10.0.1.10:5000"
```

```
# /etc/hosts (all nodes)
10.0.1.10  registry.internal
```

Kubelet uses `registries.yaml` to resolve `registry.internal:5000` when pulling images. In-cluster pods (Temporal worker) use the IP directly via env var.

---

## 12. Ansible

```
infra/
├── ansible/
│   ├── inventory/
│   │   └── hosts.yml
│   ├── group_vars/
│   │   ├── all.yml
│   │   ├── ops.yml
│   │   ├── build.yml
│   │   └── run.yml
│   ├── playbooks/
│   │   ├── site.yml
│   │   ├── add-run-node.yml
│   │   └── upgrade-k3s.yml
│   └── roles/
│       ├── common/          # SSH hardening, packages, fail2ban, NTP
│       ├── vswitch/         # Netplan VLAN for dedicated servers
│       ├── k3s_server/      # k3s server install (--cluster-init)
│       ├── k3s_agent/       # k3s agent join (taint flag conditional on group)
│       ├── gvisor/          # runsc install, containerd config
│       ├── registry_client/ # registries.yaml + /etc/hosts
│       └── firewall/        # iptables: block metadata, SMTP, registry port
└── k8s/
    ├── dp-system.yml
    ├── temporal-worker.yml
    ├── registry.yml
    ├── registry-gc.yml
    ├── buildkit.yml
    ├── cert-manager-issuer.yml
    ├── wildcard-cert.yml
    ├── traefik-tlsstore.yml
    ├── gvisor-runtimeclass.yml
    ├── loki-ingress.yml
    ├── grafana-ingress.yml
    ├── loki-values.yml
    ├── prometheus-values.yml
    └── grafana-values.yml
```

### Inventory

```yaml
all:
  vars:
    ansible_user: root
    k3s_version: "v1.31.4+k3s1"
    vswitch_vlan_id: 4001
    registry_ip: "10.0.1.10"
    apps_domain: "breacher.org"

  children:
    ctrl:
      hosts:
        k3s-1:
          ansible_host: <public-ip>
          private_ip: 10.0.1.1
          is_dedicated: false

    ops:
      hosts:
        ops-1:
          ansible_host: <public-ip>
          private_ip: 10.0.1.10
          is_dedicated: true
          physical_nic: enp0s31f6

    build:
      hosts:
        build-1:
          ansible_host: <public-ip>
          private_ip: 10.0.1.2
          is_dedicated: false

    run:
      hosts:
        run-1:
          ansible_host: <public-ip>
          private_ip: 10.0.1.100
          is_dedicated: true
          physical_nic: enp0s31f6

    dedicated:
      children:
        ops:
        run:
```

### add-run-node.yml

```yaml
- hosts: "{{ target | default(ansible_limit) }}"
  become: true
  roles:
    - common
    - vswitch
    - k3s_agent # joins WITHOUT taint (run group has no taint in group_vars)
    - gvisor
    - registry_client
    - firewall
  post_tasks:
    - name: Verify node joined
      command: kubectl get node {{ inventory_hostname }} -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'
      delegate_to: k3s-1
      register: node_status
      retries: 12
      delay: 10
      until: node_status.stdout == "True"
    - name: Reminder
      debug:
        msg: "{{ inventory_hostname }} is Ready. Add A record: *.{{ apps_domain }} → {{ ansible_host }}"
```

---

## 13. Security Summary

| Layer           | Protection                                                                             |
| --------------- | -------------------------------------------------------------------------------------- |
| Runtime         | gVisor (runsc) — syscall filtering                                                     |
| Pod security    | non-root, drop ALL caps, no privilege escalation, no SA token                          |
| Network ingress | Default-deny, allow only same-namespace + Traefik                                      |
| Network egress  | Default-deny, allow DNS + public internet, block 10.0.0.0/8 + 172.16.0.0/12 + metadata |
| Registry        | hostPort firewalled, only 10.0.1.0/24                                                  |
| BuildKit        | Only reachable via cluster DNS from dp-system (worker)                                 |
| Temporal        | No secrets in workflow history, tokens minted inside activities                        |
| Quotas          | Per-project ResourceQuota: 8 CPU, 8Gi, 20 pods                                         |

---

## 14. MCP Tool Schema

Unchanged from previous version. Rename `app` → `service`. Memory k8s-native. `railpack` default. `gitea` host. Project auto-creates, defaults to `"default"`. `name` is globally unique and becomes the subdomain.

_(Full Go types omitted for brevity — see previous version, no changes.)_

---

## 15. Adding a New Run Node

```bash
# 1. Buy Hetzner Auction server, attach to Robot vSwitch

# 2. Add to inventory under run.hosts

# 3. ansible-playbook playbooks/add-run-node.yml --limit run-2

# 4. Add DNS A record: *.breacher.org → <run-2-ip>
```

---

## 16. Verification Checklist

| What              | Test                                                |
| ----------------- | --------------------------------------------------- |
| Private network   | All nodes ping 10.0.1.x                             |
| k3s cluster       | `kubectl get nodes` — 4 nodes, correct labels       |
| Registry          | Push + pull over private network                    |
| Registry firewall | `curl <ops-1-public-ip>:5000` times out             |
| BuildKit          | Build test app, push to registry                    |
| Traefik + TLS     | `curl https://test.breacher.org`                    |
| gVisor            | Customer pod: `dmesg \| grep gVisor`                |
| Temporal worker   | Connects to Temporal Cloud, completes test workflow |
| Loki auth         | `curl https://loki.breacher.org/ready` with auth    |
| Grafana           | `ops.breacher.org` shows all node health            |
| Egress blocked    | Customer pod can't reach 10.0.1.10:5000             |
| `create_service`  | Agent deploys → URL returns 200                     |
| `get_service`     | Logs returned from Loki                             |
| `redeploy`        | Webhook → new version live                          |
| `delete_service`  | k8s objects cleaned                                 |
| Cached rebuild    | Code-only change < 15s                              |
| Add run node      | Playbook → Ready in < 15 min                        |

---

## 17. v1 Notes

1. Variable references — `${{resource.DATABASE_URL}}`
2. Custom domains — CNAME + cert-manager per-domain cert
3. Persistent volumes
4. Blue/green deploys
5. Autoscaling + scale-to-zero
6. Multi-region clusters
7. k3s HA (k3s-2, k3s-3)
8. Switch to `*.ml.ink`
9. Gitea on ops-1
