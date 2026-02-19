# eu-central-1 — Hetzner, FSN1

Production cluster on Hetzner Cloud VPS + Dedicated servers in Falkenstein, Germany.

## Machines

Source of truth for all hosts. IPs also live in `inventory/hosts.yml` for Ansible.

| Name                    | IPv4            | Private IP | Type                          | Role                             | SSH                        |
| ----------------------- | --------------- | ---------- | ----------------------------- | -------------------------------- | -------------------------- |
| k3s-1                   | 46.225.100.234  | 10.0.0.4   | Cloud VPS CX33 (8GB)          | K3s control plane                | `ssh root@46.225.100.234`  |
| build-1                 | 46.225.92.127   | 10.0.0.3   | Cloud VPS (4 vCPU, 16GB)      | BuildKit image builds            | `ssh root@46.225.92.127`   |
| ops-1                   | 116.202.163.209 | 10.0.1.4   | Dedicated (Xeon E-2176G)      | Registry, git-server, monitoring | `ssh root@116.202.163.209` |
| run-1                   | 157.90.130.187  | 10.0.1.3   | Dedicated (EPYC 7502P, 256GB) | Customer workloads               | `ssh root@157.90.130.187`  |
| dns-eu-1                | 46.225.65.56    | 10.0.0.2   | Cloud VPS CX22 (4GB)          | PowerDNS authoritative DNS       | `ssh root@46.225.65.56`    |
| load-balancer-central-1 | 46.225.35.234   | 10.0.0.5   | Hetzner LB (lb11)             | Custom domain TCP passthrough    | Hetzner Console only       |

### Notable hardware

**run-1**: 256GB ECC RAM, 2x 1.92TB NVMe. Handles all customer pods — sized for density.

**ops-1**: 2x 960GB NVMe (RAID1 → `/` + `/data`) + 2x 2TB HDD (RAID1 → `/backups`). Hosts registry + git-server with built-in redundancy.

**k3s-1**: Only 8GB — control plane only. Customer pods MUST NOT land here (gVisor RuntimeClass prevents this via nodeSelector).

## Network

Hetzner Cloud Network + vSwitch bridge cloud VPS and dedicated servers into one private network.

| Setting            | Value       |
| ------------------ | ----------- |
| Cloud Network ID   | 11898981    |
| vSwitch VLAN ID    | 4000        |
| Subnet (dedicated) | 10.0.1.0/24 |
| Subnet (cloud)     | 10.0.0.0/16 |
| Gateway            | 10.0.1.1    |

**How it connects**: Cloud VPS nodes attach to the Cloud Network via Hetzner Console. Dedicated servers use vSwitch VLAN 4000 via netplan (managed by the `vswitch` Ansible role). The Hetzner LB attaches to the Cloud Network.

## Cluster topology

Four node pools isolated by taints. This prevents build jobs or platform services from competing with customer traffic.

| Pool  | Node    | Taint                   | What runs here                                         | Density          |
| ----- | ------- | ----------------------- | ------------------------------------------------------ | ---------------- |
| ctrl  | k3s-1   | —                       | K3s API, Helm controllers, cert-manager                | default (110)    |
| ops   | ops-1   | `pool=ops:NoSchedule`   | Docker Registry, git-server, Grafana, Prometheus, Loki | default (110)    |
| build | build-1 | `pool=build:NoSchedule` | BuildKit builders                                      | default (110)    |
| run   | run-1   | —                       | Customer pods (gVisor sandboxed), Traefik ingress      | max-pods=800     |

**Why dedicated for run/ops**: Run needs 256GB RAM for pod density. Ops needs RAID storage for registry + git repo durability. Cloud VPS is fine for control plane and builds.

## Traffic flow

### Standard domains (`*.ml.ink`)

```
Client → Cloudflare LB → run-pool (Traefik, TLS via wildcard cert) → customer pod
```

Cloudflare handles DDoS, health-checks, and TLS termination (full strict mode).

### Custom domains (DNS delegation)

Users delegate a subdomain zone once via NS records. The platform controls the zone via PowerDNS (dns-eu-1), issues a wildcard cert via DNS-01, and creates subdomains instantly.

```
1. User calls delegate_zone("apps.example.com")
   → Returns TXT verification instructions

2. User calls verify_delegation (phase 1: TXT ownership proof)
   → Returns NS delegation instructions (ns1.ml.ink, ns2.ml.ink)

3. User calls verify_delegation (phase 2: NS check)
   → Temporal workflow:
     a. Create zone in PowerDNS (SOA + NS + wildcard A record)
     b. Issue wildcard cert *.apps.example.com via DNS-01 (RFC2136 → PowerDNS)
     c. Zone status → active

4. User calls add_custom_domain("api", service_name)
   → Create A record in PowerDNS + Ingress referencing wildcard cert
   → Live in seconds

Traffic flow:
api.apps.example.com
  → Recursive resolver → NS ns1.ml.ink → PowerDNS (46.225.65.56)
    → A 46.225.35.234
  → Hetzner LB → TCP passthrough → Traefik (run-1)
  → Traefik routes by Host header → customer pod
```

**PowerDNS**: Authoritative DNS on dns-eu-1 (46.225.65.56 / 10.0.0.2). API on private network only (8081). Local PostgreSQL backend.

**TLS**: Wildcard cert per delegated zone via DNS-01 (cert-manager RFC2136 solver → PowerDNS). No per-service certs.

**Why TCP passthrough**: Traefik terminates TLS using wildcard certs from cert-manager. TLS-terminating LB would break this.

**Anti-squat**: Unverified zones expire after 7 days. TXT verification while user still controls DNS prevents unauthorized claiming.

## Hetzner Load Balancer

Managed by the `hetzner_lb` Ansible role (runs as part of `site.yml`). Uses Hetzner Cloud API via `hcloud` CLI.

| Setting    | Value                                     |
| ---------- | ----------------------------------------- |
| Name       | load-balancer-central-1                   |
| Type       | lb11                                      |
| Location   | fsn1                                      |
| Public IP  | 46.225.35.234                             |
| Private IP | 10.0.0.5                                  |
| Network    | Cloud Network (subnet 10.0.0.0/24)        |
| Targets    | Run-pool node private IPs (e.g. 10.0.1.3) |

### Services

| Listen Port | Destination Port | Protocol | Mode            |
| ----------- | ---------------- | -------- | --------------- |
| 80          | 80               | TCP      | —               |
| 443         | 443              | TCP      | TLS Passthrough |

Port 443 **must** use TLS Passthrough (not TLS Termination). Traefik on run-1 terminates TLS using per-domain certs from cert-manager. If the LB terminates TLS, cert-manager HTTP-01 challenges and per-domain cert routing both break.

The Ansible `hetzner_lb` role creates both services via `hcloud load-balancer add-service --protocol tcp`. TLS Passthrough is the Hetzner Console name for TCP protocol on port 443.

**Hetzner-specific**: Other providers will need an equivalent TCP passthrough LB. The `hetzner_lb` role won't apply to non-Hetzner regions.

**Requires**: `hcloud` CLI on control node + `HCLOUD_TOKEN` via vault or `--extra-vars`.

## Firewall hardening

Run-pool nodes restrict ports 80/443 to known sources only (configured in `inventory/group_vars/run.yml`):

- `46.225.35.234/32` — Hetzner LB (custom domain traffic)
- 15 Cloudflare IPv4 CIDRs — `*.ml.ink` traffic

Everything else on 80/443 is DROPped. All nodes also block metadata endpoint (169.254.169.254) and SMTP egress (25, 465, 587).

## Internal registry

Container images for all customer deployments are stored in a plain HTTP registry on ops-1.

| Setting      | Value                                                           |
| ------------ | --------------------------------------------------------------- |
| Host         | ops-1 (10.0.1.4:5000)                                           |
| K8s service  | registry.dp-system:5000                                         |
| DNS alias    | registry.internal:5000                                          |
| Storage      | hostPath `/mnt/registry` on ops-1 (RAID1 NVMe)                  |
| Protocol     | HTTP (plain, private network only)                              |
| Image format | `registry.internal:5000/dp-{user_id}-{project}/{service}:{sha}` |

Registry access is restricted to the private network via firewall (iptables DROP for public access on ops nodes). A registry-gc CronJob handles garbage collection.

## Observability

All observability runs on ops-1 via Helm charts deployed by the `k8s_addons` role.

| Component    | Chart Version                 | Access                        | Retention |
| ------------ | ----------------------------- | ----------------------------- | --------- |
| Prometheus   | kube-prometheus-stack 81.6.1  | prometheus.ml.ink (BasicAuth) | 30d       |
| Loki         | grafana/loki 6.53.0           | loki.ml.ink (BasicAuth)       | 30d       |
| Grafana      | grafana/grafana 10.5.15       | grafana.ml.ink                | —         |
| Promtail     | grafana/promtail 6.17.1       | — (log shipper)               | —         |
| Traefik      | traefik/traefik 39.0.0        | — (ingress controller)        | —         |
| cert-manager | jetstack/cert-manager v1.19.3 | — (TLS automation)            | —         |

Chart versions are pinned in `inventory/group_vars/all/main.yml`. Helm values in `k8s/values/`.

## Scaling

**Current density**: Run nodes are tuned for max-pods=800 with parallel image pulls. With gVisor overhead (64Mi reserved per pod), a 256GB run node fits ~500-800 customer pods depending on workload memory. See `infra/README.md` § "Kubelet density tuning" for the full settings table.

**Node CIDR**: Controller-manager allocates /22 subnets (1022 IPs per node) to new nodes. Existing nodes may still have /24 from before the change — delete and rejoin the node object to get /22 if you need >254 pods. With /16 cluster CIDR and /22 per node, the cluster supports up to 64 nodes.

**Add run capacity** (common): Provision a new dedicated server → add to inventory under `run` → run `add-run-node.yml`. The playbook joins k3s, installs gVisor, configures firewall, adds to Hetzner LB. Manually add to Cloudflare LB. New run nodes automatically inherit the density config from `k3s_agent_kubelet_args` in the run group vars.

**Add build capacity** (rare): Same process but under `build` group with `pool=build:NoSchedule` taint. Enables parallel BuildKit builders.

**Scaling ops is complex**: Registry, git-server, and observability are stateful hostPath workloads on ops-1. Scaling would require migrating to shared storage or running replicated services. Not needed at current scale.

**No auto-scaling**: All nodes managed manually via Ansible.

## K8s manifests

```
k8s/
├── system/          Namespaces, RBAC, cert-manager issuers, wildcard cert, TLS store
├── workloads/       BuildKit, deployer-server, deployer-worker, git-server, registry
├── observability/   Grafana, Loki, Prometheus ingresses
├── values/          Helm chart value overrides (Traefik, cert-manager, Loki, etc.)
└── templates/       Customer resource design specs
```

Templates are the contract between infra and Go code:

- `customer-namespace-template.yml` — namespace + resource quota
- `customer-service-template.yml` — secret, deployment, service, default ingress
- `customer-custom-domain-template.yml` — custom domain ingress (TLS via pre-provisioned Certificate CR)

## Applying changes

Dry-run (shows what would change, no modifications):

```bash
cd infra/ansible
ansible-playbook playbooks/site.yml --check --diff
```

Apply:

```bash
ansible-playbook playbooks/site.yml
```

See `ansible/README.md` for secrets, patching, node addition, and other operations.

## Region-specific decisions

Platform-wide decisions (gVisor, firewall, SMTP blocking, etc.) are in `infra/README.md`.

| Decision                             | Rationale                                                                                                                                       |
| ------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Dedicated servers for run + ops      | Run needs 256GB RAM for pod density. Ops needs RAID storage for registry + git repo durability. Cloud VPS is fine for control plane and builds. |
| Hetzner LB for custom domain traffic | Region-specific implementation of the global TCP passthrough requirement. Other regions will use their provider's equivalent.                   |
