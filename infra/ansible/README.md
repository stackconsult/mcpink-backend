# Ansible Automation

Operations runbook for all MCPDeploy clusters. Region-specific details (machines, network, topology) live in `infra/<region>/README.md`.

## Layout

```
playbooks/
├── site.yml                Full cluster bootstrap/reconcile
├── add-run-node.yml        Add or reconcile a single run node
├── upgrade-k3s.yml         Rolling k3s upgrade
├── patch-hosts.yml         OS security updates (apt upgrade + optional reboot)
└── refresh-known-hosts.yml Rebuild per-cluster known_hosts from inventory

roles/
├── common                  Base packages, kernel modules, sysctl, disable swap
├── vswitch                 VLAN netplan for dedicated servers
├── k3s_server              K3s control plane install/upgrade
├── k3s_agent               K3s agent install/upgrade
├── gvisor                  gVisor runtime for containerd (run nodes)
├── registry_client         kubelet registry mirror + containerd certs.d
├── firewall                iptables rules (ingress restrictions, egress blocks)
├── k8s_addons              Helm charts + K8s secrets + RBAC + quota reconciler
├── hetzner_lb              Hetzner Cloud LB management (hcloud CLI, Hetzner-specific)
└── security_patch          apt upgrade + conditional reboot
```

## Prerequisites

1. `ansible-core` installed locally
2. SSH access as root to all target nodes
3. Private networking configured (provider-specific — see region README)
4. DNS records in place (see `infra/README.md`)

## SSH host keys

Host key checking is enforced. Each cluster has its own `known_hosts` file (e.g., `eu-central-1/known_hosts`), referenced via `ansible_ssh_extra_args` in group_vars.

```bash
ansible-playbook playbooks/refresh-known-hosts.yml
```

Run this whenever you add/reimage hosts or rotate SSH keys.

## Important: Ansible is the source of truth

All long-lived infrastructure changes (firewall rules, K8s manifests, Helm charts, secrets, node configuration) MUST go through Ansible — not manual `kubectl apply` or SSH commands. One-off debugging is fine, but any change that should persist across reprovisioning belongs in a role or manifest committed to this repo.

```bash
# Dry-run first, always
ansible-playbook playbooks/site.yml --check --diff
# Then apply
ansible-playbook playbooks/site.yml --diff
```

## Bootstrap (first run)

```bash
cd infra/ansible
ansible-playbook playbooks/refresh-known-hosts.yml
ansible-playbook playbooks/site.yml
```

`site.yml` runs in order: baseline all nodes → k3s server → k3s agents + gVisor → Helm addons → Hetzner LB.

Secrets can live in vault or be passed at runtime:

```bash
ansible-playbook playbooks/site.yml \
  -e cloudflare_api_token="$CLOUDFLARE_API_TOKEN" \
  -e loki_basic_auth_users="deploy:$LOKI_BCRYPT"
```

**Manual step after bootstrap**: Add run-pool node IPs to the Cloudflare LB origin pool (Cloudflare dashboard).

## Day-2 operations

### Add a run node

1. Provision server in Hetzner, add to `inventory/hosts.yml` under `run`
2. `ansible-playbook playbooks/add-run-node.yml --limit <hostname>`
3. Node is auto-added to Hetzner LB by the playbook
4. Manually add to Cloudflare LB origin pool

### Security patching

```bash
ansible-playbook playbooks/patch-hosts.yml
```

Options: `-e security_patch_reboot_if_required=false` (no reboot), `-e serial=2` (batch size).

### K3s upgrade

Update `k3s_version` in inventory, then:

```bash
ansible-playbook playbooks/upgrade-k3s.yml
```

## Secrets (vault-managed)

`site.yml` creates K8s secrets when the corresponding vars are set.

| K8s Secret | Namespace | Vault variable(s) |
|------------|-----------|-------------------|
| `github-app` | dp-system | `github_app_id`, `github_app_private_key` |
| `temporal-creds` | dp-system | `temporal_cloud_api_key` |
| `temporal-worker-config` | dp-system | `temporal_address`, `temporal_namespace` |
| `cloudflare-api-token` | cert-manager | `cloudflare_api_token` |
| `loki-auth-secret` | dp-system | `loki_basic_auth_users` |
| `ghcr-pull-secret` | dp-system | `ghcr_pull_token` |

## CI/CD deploy user

The `k8s_addons` role provisions a restricted `deploy` user on the control plane for GitHub Actions:

- Linux user `deploy` with SSH key from `deploy_ssh_private_key`
- RBAC-limited ServiceAccount (`ci-deployer`) — can only `get/patch` deployer deployments in `dp-system`
- Restricted kubeconfig at `/home/deploy/.kube/config`

### Vault variables

```yaml
# eu-central-1/inventory/group_vars/all/secrets.yml

# ssh-keygen -t ed25519 -f deploy_key -N "" -C "ci-deploy"
deploy_ssh_private_key: |
  -----BEGIN OPENSSH PRIVATE KEY-----
  ...
  -----END OPENSSH PRIVATE KEY-----

# GitHub → Settings → Developer settings → Personal access tokens (read:packages)
ghcr_pull_token: "ghp_..."
```

### GitHub Actions secrets

| Secret | Value |
|--------|-------|
| `K3S_HOST` | Control plane public IP (see region README) |
| `DEPLOY_SSH_KEY` | Same as `deploy_ssh_private_key` in vault |
