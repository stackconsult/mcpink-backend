# Migration: Personal Account → Business Account

Fresh infrastructure on new Hetzner business account. Same architecture, same env vars, clean slate (no data migration).

## Key Changes from Previous Setup

| Change             | Old               | New                                    |
| ------------------ | ----------------- | -------------------------------------- |
| Control plane name | k3s-1             | **ctrl-1**                             |
| Build node type    | Cloud VPS         | **Dedicated server**                   |
| Registry location  | ops-1             | **build-1** (co-located with BuildKit) |
| Gitea              | Deployed on ops-1 | **Removed** (replaced by git-server)   |
| Hetzner account    | Personal          | **Business**                           |
| All IPs            | Old               | **New** (see inventory below)          |

## Region

All dedicated servers are in **FSN (Falkenstein)**. ctrl-1 and powerdns-1 are temporarily in **NBG (Nuremberg)** — CX22 was unavailable in FSN at time of provisioning. This is fine: Cloud Networks span the entire `eu-central` zone (FSN + NBG + HEL), so the VPS nodes reach dedicated servers over the private network without issue. The VPS nodes use Cloud Network, not vSwitch.

**TODO**: Recreate ctrl-1 and powerdns-1 in FSN once CX22 availability returns. Delete NBG instances, create new ones in FSN, update IPs in inventory, and re-run Ansible. No data to migrate — ctrl-1 state is in etcd (re-bootstrap k3s), powerdns-1 is stateless (config from Ansible).

## New Server Inventory

### Cloud VPS (NBG — temporary)

| Name     | Public IPv4 | Private IP | Type       | Pool | Private Iface |
| -------- | ----------- | ---------- | ---------- | ---- | ------------- |
| ctrl-1   | 46.224.206.46 | 10.0.0.3 | CX22 (4GB) | ctrl | enp7s0        |
| powerdns-1 | 46.225.84.41 | 10.0.0.4 | CX22 (4GB) | dns  | enp7s0        |

### Dedicated Servers (FSN)

| Name    | Public IPv4                | Private IP | Type                                           | Pool  | Physical NIC           | Private Iface |
| ------- | -------------------------- | ---------- | ---------------------------------------------- | ----- | ---------------------- | ------------- |
| build-1 | 78.46.80.236               | 10.0.1.5   | i9-13900, 64GB ECC, 2x1.92TB NVMe DC           | build | enp6s0                 | vlan4000      |
| ops-1   | 176.9.150.48               | 10.0.1.4   | Ryzen 9 5950X, 128GB ECC, 2x3.84TB U.2 NVMe DC | ops   | enp41s0                | vlan4000      |
| run-1   | 148.251.156.12             | 10.0.1.3   | EPYC 7502P, 448GB ECC, 1x960GB U.2 NVMe DC     | run   | enp195s0               | vlan4000      |

### Networking

| Resource              | Value                               |
| --------------------- | ----------------------------------- |
| Cloud Network ID      | 11948890                            |
| Cloud Network Subnet  | 10.0.0.0/16                         |
| vSwitch VLAN ID       | 4000                                |
| Dedicated Subnet      | 10.0.1.0/24                         |
| Private Gateway       | 10.0.1.1                            |
| Hetzner LB Public IP  | 49.12.19.38                         |
| Hetzner LB Private IP | 10.0.0.2                            |
| Hetzner LB Name       | lb-eu-central-1                     |
| vSwitch ID            | 77471                               |

### Hetzner Firewalls

**Lesson learned**: On the old personal account, the iptables INPUT chain had no default-deny policy. BSI CERT-Bund flagged rpcbind (port 111/udp) publicly accessible on k3s-1, ops-1, and run-1 (2026-02-10). For the new infra, configure Hetzner-level firewalls as the outer layer **before** any services are deployed.

#### Cloud Firewall (Hetzner Console — Cloud VPS nodes)

| Firewall | Applied To | Inbound ALLOW Rules                           |
| -------- | ---------- | --------------------------------------------- |
| fw-ctrl  | ctrl-1     | TCP 22 (SSH), ICMP                            |
| fw-dns   | powerdns-1 | TCP 22 (SSH), ICMP, UDP 53 (DNS), TCP 53 (DNS) |

Default policy: DROP all other inbound. Private network traffic (Cloud Network / vSwitch) bypasses Cloud Firewalls.

#### Robot Firewall (Hetzner Robot panel — Dedicated servers)

| Firewall | Applied To | Inbound ALLOW Rules                              |
| -------- | ---------- | ------------------------------------------------ |
| fw-build | build-1    | TCP 22 (SSH)                                     |
| fw-ops   | ops-1      | TCP 22 (SSH)                                     |
| fw-run   | run-1      | TCP 22 (SSH), TCP 80 (HTTP), TCP 443 (HTTPS)     |

Default policy: DROP all other inbound. vSwitch traffic bypasses Robot Firewalls.

**Notes**:
- All inter-node traffic (k3s API 6443, kubelet, flannel VXLAN, registry 5000, metrics 9100) uses private network (10.0.0.0/16) — no public firewall rules needed
- run-1 allows 80/443 publicly because Hetzner LB and Cloudflare connect directly. Host iptables further restricts to LB + Cloudflare source IPs only
- Host-level iptables (Ansible firewall role) is the inner layer with the same default-deny + source restrictions

### Finding Physical NIC Names

SSH into each dedicated server and run:

```bash
ip link show | grep -E '^[0-9]+:' | grep -v 'lo\|docker\|veth\|br-\|flannel\|cni'
```

Common results: `eno1` (Intel), `enp1s0` or `enp195s0` (AMD/PCIe).

## Private IP Assignment Plan

### Cloud VPS (auto-assigned by Cloud Network)

| Node     | Private IP           |
| -------- | -------------------- |
| ctrl-1   | 10.0.0.3             |
| powerdns-1 | 10.0.0.4           |
| LB       | 10.0.0.2 (confirmed) |

### Dedicated (manual via vSwitch netplan)

| Node    | Private IP | Physical NIC | VLAN Iface |
| ------- | ---------- | ------------ | ---------- |
| build-1 | 10.0.1.5   | enp6s0       | enp6s0.4000 → vlan4000 |
| ops-1   | 10.0.1.4   | enp41s0      | enp41s0.4000 → vlan4000 |
| run-1   | 10.0.1.3   | enp195s0     | enp195s0.4000 → vlan4000 |

vSwitch config (handled by Ansible `vswitch` role via netplan):
- VLAN interface: `{nic}.4000` with MTU 1400
- IP: `10.0.1.x/24`
- Route: `10.0.0.0/16` via `10.0.1.1`

## Provisioning Checklist

### Phase 1: Hetzner Networking

- [ ] Create Cloud Network in Hetzner Console (zone: eu-central, subnet: 10.0.0.0/16)
- [ ] Record Cloud Network ID
- [ ] Attach ctrl-1 and powerdns-1 to Cloud Network
- [ ] Create vSwitch with VLAN ID 4000 in Hetzner Robot
- [ ] Attach build-1, ops-1, run-1 to vSwitch
- [ ] Attach Hetzner LB to Cloud Network
- [ ] Configure LB: TCP services on port 80→80 and 443→443 (TLS passthrough)
- [ ] Add run-1 private IP (10.0.1.3) as LB target

### Phase 1b: Hetzner Firewalls

Set up **before** deploying any services. Default-deny from day one.

- [ ] Create Cloud Firewalls in Hetzner Console (fw-ctrl, fw-build, fw-dns) — see rules in "Hetzner Firewalls" section above
- [ ] Apply fw-ctrl to ctrl-1, fw-build to build-1, fw-dns to powerdns-1
- [ ] Create Robot Firewalls in Hetzner Robot panel (fw-ops, fw-run) — see rules in "Hetzner Firewalls" section above
- [ ] Apply fw-ops to ops-1, fw-run to run-1
- [ ] Verify: port scan each node from external host to confirm only allowed ports are open

### Phase 2: OS Bootstrap

For each dedicated server:

- [x] ops-1: installimage Ubuntu 24.04, RAID1 on 2x3.84TB NVMe, NIC: `enp41s0`
  - RAID1 verified: all md devices [UU], 3.4TB on /mnt
  - Partitions: swap 4G, /boot 1G, / 100G, /mnt 3.4TB (all)
  - Data dirs created: /mnt/git-repos, /mnt/prometheus, /mnt/loki, /mnt/grafana
- [x] build-1: installimage Ubuntu 24.04, RAID1 on 2x1.92TB U.2 NVMe DC, NIC: `enp6s0`
  - RAID1 verified: all md devices [UU], 1.6TB on /mnt
  - Partitions: /boot/efi 256M, swap 4G, /boot 1G, / 100G, /mnt 1.6TB (all)
  - Data dirs created: /mnt/buildkit, /mnt/registry
- [x] run-1: installimage Ubuntu 24.04, single NVMe (894GB), no RAID, NIC: `enp195s0`
  - Partitions: /boot 1G, / 893G. No /mnt partition (stateless node — not needed)
- [x] SSH in to each, run `ip link show` to find physical NIC names
  - ops-1: `enp41s0`, build-1: `enp6s0`, run-1: `enp195s0`
- [x] Record NIC names in this doc and inventory

### Phase 3: Update Ansible Inventory

Update `infra/eu-central-1/inventory/hosts.yml` with all new IPs and NIC names.

See "Code/Manifest Changes Required" section below for all files to update.

### Phase 4: Ansible Run

```bash
cd infra/ansible

# Dry-run first
ansible-playbook playbooks/site.yml --check --diff

# Apply
ansible-playbook playbooks/site.yml
```

### Phase 5: DNS / Cloudflare

- [ ] Update Cloudflare LB origin pool: add run-1 public IP
- [ ] Verify `*.ml.ink` resolves through Cloudflare LB
- [ ] Update `cname.ml.ink` A record → 49.12.19.38
- [ ] Update `*.cname.ml.ink` A record → 49.12.19.38
- [ ] Update `ns1.ml.ink` / `ns2.ml.ink` A records → powerdns-1 new public IP
- [ ] Verify PowerDNS responds on powerdns-1

### Phase 6: Verify

- [ ] `kubectl get nodes` — all nodes joined
- [ ] `kubectl get pods -n dp-system` — all workloads running
- [ ] Test deploy: `create_service` via MCP
- [ ] Test custom domain flow
- [ ] Prometheus/Loki/Grafana accessible

## Code/Manifest Changes Required

### 1. `inventory/hosts.yml`

- Rename `k3s-1` → `ctrl-1`
- Update all public and private IPs
- build-1: `is_dedicated: true`, add `physical_nic`, `private_iface: vlan4000`
- Add `build` to `dedicated.children`
- Update `registry_ip` to build-1's private IP (10.0.1.5)

### 2. `inventory/group_vars/all/main.yml`

```yaml
hetzner_lb_public_ip: 49.12.19.38
hetzner_lb_private_ip: 10.0.0.2
hetzner_lb_name: lb-eu-central-1
```

### 2a. Refactor: remove `hetzner_lb` Ansible role

The `hetzner_lb` role manages the LB via `hcloud` CLI + API token. Since we manage the LB
through Hetzner Console manually, this role is dead code. Remove:

- `ansible/roles/hetzner_lb/` — entire role
- `hetzner_cloud_network_id` from `group_vars/all/main.yml` — only used by this role
- `hcloud_token` references from vault/extra-vars
- Remove the role from `playbooks/site.yml` (or wherever it's included)

### 3. `inventory/group_vars/build.yml`

Add registry storage path (registry now on build node):

```yaml
# Add:
registry_storage_path: /mnt/registry
```

### 4. `inventory/group_vars/ops.yml`

Remove registry, remove gitea, add git-server:

```yaml
---
git_server_storage_path: /mnt/git-repos
loki_storage_size: 50Gi
grafana_storage_size: 10Gi
prometheus_retention: 30d
```

### 5. `inventory/group_vars/run.yml`

Update LB IP in allowed sources:

```yaml
traefik_public_allowed_sources:
  - 49.12.19.38/32 # NEW Hetzner LB
  # Cloudflare IPv4 ranges — unchanged
  - 173.245.48.0/20
  - 103.21.244.0/22
  - 103.22.200.0/22
  - 103.31.4.0/22
  - 141.101.64.0/18
  - 108.162.192.0/18
  - 190.93.240.0/20
  - 188.114.96.0/20
  - 197.234.240.0/22
  - 198.41.128.0/17
  - 162.158.0.0/15
  - 104.16.0.0/13
  - 104.24.0.0/14
  - 172.64.0.0/13
  - 131.0.72.0/22
```

### 6. Firewall role: `ansible/roles/firewall/tasks/main.yml`

Registry firewall rules: change `'ops' in group_names` → `'build' in group_names`

### 7. K8s: `k8s/workloads/registry.yml`

Change nodeSelector and toleration from ops → build:

```yaml
nodeSelector:
  pool: build # was: ops
tolerations:
  - key: pool
    value: build # was: ops
    effect: NoSchedule
```

### 8. K8s: Remove Gitea

Delete:

- `k8s/workloads/gitea.yml`
- `k8s/workloads/gitea-ingress.yml`

### 9. Known hosts

Regenerate after all servers are up:

```bash
ssh-keyscan -H <all-public-ips> > infra/eu-central-1/known_hosts
```

## Storage Layout

### ops-1 (RAID1: 2x 3.84TB NVMe = 3.84TB usable)

```
/               — OS + system
/mnt/git-repos  — bare git repositories (git-server)
/mnt/prometheus — Prometheus TSDB (30d retention)
/mnt/loki       — Loki log chunks (30d retention)
/mnt/grafana    — Grafana dashboards/state
```

### build-1 (RAID1: 2x 1.92TB NVMe = 1.92TB usable)

```
/               — OS
/mnt/buildkit   — BuildKit layer cache
/mnt/registry   — Docker Registry v2 images
```

Registry co-located with BuildKit for fast push after build.

### run-1 (1x 960GB U.2 NVMe)

```
/               — OS + containerd image cache
```

Stateless — all customer pods are ephemeral.

## Firewall Hardening (Ansible)

### Host-level iptables changes for new infra

- [ ] Add default-deny: append `iptables -A INPUT -j DROP` as the **last rule** in the INPUT chain
- [ ] Disable rpcbind on all nodes: `systemctl disable --now rpcbind rpcbind.socket && systemctl mask rpcbind rpcbind.socket`

These are changes to the Ansible firewall role (`ansible/roles/firewall/tasks/main.yml`). The old infra lacked the default-deny — everything not explicitly ACCEPTed fell through to ACCEPT policy, which is how rpcbind (port 111) ended up exposed.

## Rollback

No rollback needed — clean-slate deployment. Old servers on personal account remain running until new cluster is verified, then decommissioned.
