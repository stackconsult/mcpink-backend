# k3s Ansible Automation

This directory implements `infra/k3s/ARCHITECTURE_PLAN.md` as reproducible Ansible.

It also preserves the original `README.md` platform goals:
- Stable agent contract (`create/redeploy/get/delete`) while swapping infra internals.
- 3-plane isolation (control/build/run) to prevent build contention on runtime nodes.
- Security defaults (gVisor runtime class, egress restrictions, host firewall, no workflow secrets).

## Layout

- `inventory/hosts.yml` - Host inventory and global variables.
- `inventory/group_vars/` - Pool-specific defaults.
- `playbooks/site.yml` - Full cluster bootstrap/reconcile.
- `playbooks/add-run-node.yml` - Add a new run node.
- `playbooks/upgrade-k3s.yml` - Upgrade existing cluster nodes.
- `playbooks/patch-hosts.yml` - Apply OS security updates (apt upgrade + optional reboot).
- `playbooks/refresh-known-hosts.yml` - Rebuild `inventory/known_hosts` from current inventory host targets.
- `roles/` - Node/bootstrap responsibilities.
- `inventory/known_hosts` - Repository-managed SSH host keys used for strict host verification.

## Prerequisites

1. `ansible-core` installed on your local machine.
2. SSH access as root to all target nodes.
3. Hetzner private networking configured (cloud network + vSwitch).
4. DNS records in place for:
   - `*.ml.ink`
   - `grafana.ml.ink`
   - `loki.ml.ink`

## SSH host key workflow

Host key checking is enforced and SSH only trusts keys in `inventory/known_hosts`.

Refresh host keys whenever you add/reimage hosts or rotate SSH host keys:

```bash
ansible-playbook playbooks/refresh-known-hosts.yml
```

## First Run

```bash
cd infra/ansible
ansible-playbook playbooks/refresh-known-hosts.yml
ansible-playbook playbooks/site.yml
```

If you keep secrets outside inventory, pass them at runtime:

```bash
ansible-playbook playbooks/site.yml \
  -e cloudflare_api_token="$CLOUDFLARE_API_TOKEN" \
  -e loki_basic_auth_users="deploy:$LOKI_BCRYPT"
```

After adding or replacing run nodes, update the Cloudflare Load Balancer origin pool in the dashboard.
Cloudflare remains the source of truth for public host/origin routing.

## Security Patching

Run vulnerability and package updates separately from cluster reconciliation:

```bash
ansible-playbook playbooks/patch-hosts.yml --limit all
```

Useful options:

```bash
# No reboot
ansible-playbook playbooks/patch-hosts.yml -e security_patch_reboot_if_required=false

# Patch one node at a time (default) or increase serial batch size
ansible-playbook playbooks/patch-hosts.yml -e serial=2
```

## Post-bootstrap secrets (Ansible-managed)

`playbooks/site.yml` creates/updates required runtime secrets directly when vars are set. Manual `infra/k8s/*.example.yml` secret applies are no longer used.

Managed by `roles/k8s_addons`:

- `github-app` (`dp-system`): `github_app_id` + `github_app_private_key`
- `temporal-creds` (`dp-system`): `temporal_cloud_api_key` as `cloud-api-key`
- `temporal-worker-config` (`dp-system`): `temporal_address` + `temporal_namespace`
- `cloudflare-api-token` (`cert-manager`): `cloudflare_api_token`
- `loki-auth-secret` (`dp-system`): `loki_basic_auth_users`
- `ghcr-pull-secret` (`dp-system`): `ghcr_pull_token` (docker-registry secret for ghcr.io)

After setting values in inventory/vault (or via `-e`), re-run:

```bash
ansible-playbook playbooks/site.yml
```

## CI/CD deploy user

The `k8s_addons` role provisions a restricted `deploy` Linux user on the control plane node for GitHub Actions to SSH into and run `kubectl set image`. This is fully automated — no manual setup needed.

What gets created:

- Linux user `deploy` with SSH authorized_keys derived from `deploy_ssh_private_key`
- RBAC-limited ServiceAccount (`ci-deployer`) that can only `get/patch` deployer-worker and deployer-server deployments in `dp-system`
- Restricted kubeconfig at `/home/deploy/.kube/config` using the ci-deployer token
- `ghcr-pull-secret` docker-registry secret for pulling images from ghcr.io

### Vault variables for CI/CD

Add these to `inventory/group_vars/all/secrets.yml`:

```yaml
# CI/CD deploy — SSH key for GitHub Actions
# Generate: ssh-keygen -t ed25519 -f deploy_key -N "" -C "ci-deploy"
deploy_ssh_private_key: |
  -----BEGIN OPENSSH PRIVATE KEY-----
  ...
  -----END OPENSSH PRIVATE KEY-----

# ghcr.io — pull token for deployer images
# Create at: GitHub → Settings → Developer settings → Personal access tokens
# Scope: read:packages
ghcr_pull_token: "ghp_..."
```

### GitHub Secrets

The same `deploy_ssh_private_key` value must also be added as a GitHub Actions secret:

| Secret | Value |
|--------|-------|
| `K3S_HOST` | `46.225.100.234` |
| `DEPLOY_SSH_KEY` | Same value as `deploy_ssh_private_key` in vault |
