# Ansible Vault Migration Guide

The k3s cluster needs several secrets (API tokens, TLS certs, private keys) to function. This doc defines what goes where and what needs updating.

---

## 1. Classification

### Vault (encrypted) — actual secrets

| Variable | Source | Used by |
|----------|--------|---------|
| `cloudflare_api_token` | Cloudflare dashboard | cert-manager DNS-01 solver |
| `github_app_private_key` | GitHub App settings | Temporal worker (clone repos) |
| `temporal_cloud_api_key` | Temporal Cloud dashboard → API Keys | Temporal worker (API key auth) |
| `loki_basic_auth_users` | Generate with `htpasswd` | Loki ingress auth (Railway → Loki) |
| `cloudflare_origin_cert` | Cloudflare Origin CA (SSL/TLS → Origin Server) | Backup TLS cert for `*.ml.ink` (15-year validity) |
| `cloudflare_origin_key` | Cloudflare Origin CA (same) | Private key for Origin CA cert |
| `gitea_db_password` | Generate with `openssl rand -hex 16` | Gitea PostgreSQL database password |

### Plain vars — non-sensitive config

| Variable | Value | Used by |
|----------|-------|---------|
| `github_app_id` | `2764764` | Temporal worker |
| `temporal_address` | _(e.g. `your-ns.tmprl.cloud:7233`)_ | Temporal worker |
| `temporal_namespace` | _(e.g. `your-ns`)_ | Temporal worker |

These are identifiers, not secrets. Knowing the Temporal namespace or GitHub app ID doesn't grant access.
GitHub installation IDs are not stored in cluster env vars; they are passed per workflow by the backend after reading user credentials from the metadata DB.

---

## 2. File changes

### A. Create vault file

```bash
ansible-vault create inventory/group_vars/all/secrets.yml
```

Contents:

```yaml
# Cloudflare — cert-manager DNS-01 for wildcard TLS
cloudflare_api_token: "xk8..."

# GitHub App — Temporal worker clones repos
github_app_private_key: |
  -----BEGIN RSA PRIVATE KEY-----
  ...
  -----END RSA PRIVATE KEY-----

# Temporal Cloud — API key for worker connection
temporal_cloud_api_key: "your-temporal-cloud-api-key"

# Loki — basic auth for external access (Railway → Loki)
# Generate: htpasswd -nbBC 10 deploy <password>
loki_basic_auth_users: "deploy:$2y$10$..."

# Cloudflare Origin CA — backup wildcard TLS cert for *.ml.ink
# 15-year validity, trusted only by Cloudflare edge (not browsers)
# Can replace Let's Encrypt cert if cert-manager has issues
cloudflare_origin_cert: |
  -----BEGIN CERTIFICATE-----
  ...
  -----END CERTIFICATE-----
cloudflare_origin_key: |
  -----BEGIN PRIVATE KEY-----
  ...
  -----END PRIVATE KEY-----

```

### B. Add plain vars to `inventory/hosts.yml`

Under `all.vars`, add:

```yaml
    # GitHub App (non-secret identifiers)
    github_app_id: "2764764"

    # Temporal Cloud (non-secret config)
    temporal_address: ""         # e.g. "your-ns.tmprl.cloud:7233"
    temporal_namespace: ""       # e.g. "your-ns"
```

### C. Add secret creation tasks to `roles/k8s_addons/tasks/main.yml`

Three k8s Secrets are currently created manually. Add these tasks after the `dp-system` namespace creation:

```yaml
- name: Stage GitHub App private key
  copy:
    dest: /tmp/github-app-private-key.pem
    content: "{{ github_app_private_key | default('') }}"
    mode: "0600"
  when:
    - github_app_private_key | default('') | length > 0
    - github_app_id | default('') | length > 0

- name: Create GitHub App secret
  shell: |
    set -euo pipefail
    k3s kubectl -n dp-system create secret generic github-app \
      --from-literal=app-id={{ (github_app_id | default('')) | quote }} \
      --from-file=private-key=/tmp/github-app-private-key.pem \
      --dry-run=client -o yaml | k3s kubectl apply -f -
  args:
    executable: /bin/bash
  when:
    - github_app_private_key | default('') | length > 0
    - github_app_id | default('') | length > 0

- name: Remove staged GitHub App private key
  file:
    path: /tmp/github-app-private-key.pem
    state: absent
  when:
    - github_app_private_key | default('') | length > 0
    - github_app_id | default('') | length > 0

- name: Create Temporal credentials secret
  shell: |
    set -euo pipefail
    k3s kubectl -n dp-system create secret generic temporal-creds \
      --from-literal=cloud-api-key={{ temporal_cloud_api_key | quote }} \
      --dry-run=client -o yaml | k3s kubectl apply -f -
  args:
    executable: /bin/bash
  when: temporal_cloud_api_key | default('') | length > 0

- name: Create Temporal worker config secret
  shell: |
    set -euo pipefail
    k3s kubectl -n dp-system create secret generic temporal-worker-config \
      --from-literal=temporal-address={{ (temporal_address | default('')) | quote }} \
      --from-literal=temporal-namespace={{ (temporal_namespace | default('')) | quote }} \
      --dry-run=client -o yaml | k3s kubectl apply -f -
  args:
    executable: /bin/bash
  when:
    - temporal_address | default('') | length > 0
    - temporal_namespace | default('') | length > 0
```

### D. Remove defaults from `inventory/hosts.yml`

These currently exist with empty values. After vault migration, remove them:

```yaml
# REMOVE these — they move to vault
cloudflare_api_token: ""
loki_basic_auth_users: ""
```

### E. Add vault password file

```bash
# Create password file (never commit this)
echo "your-vault-password" > .vault_pass
chmod 600 .vault_pass
```

Add to `ansible.cfg`:

```ini
[defaults]
vault_password_file = .vault_pass
```

Add to `.gitignore`:

```
infra/ansible/.vault_pass
```

---

## 3. Where secrets end up at runtime

```
Vault file (encrypted in git)
  │
  ├─ cloudflare_api_token ──► k8s Secret: cloudflare-api-token (cert-manager ns)
  ├─ github_app_private_key ──► k8s Secret: github-app (dp-system ns)
  ├─ temporal_cloud_api_key ─► k8s Secret: temporal-creds (dp-system ns)
  ├─ loki_basic_auth_users ──► k8s Secret: loki-auth-secret (dp-system ns)
  ├─ cloudflare_origin_cert ──► backup: can create k8s Secret wildcard-apps-tls
  └─ cloudflare_origin_key ───► backup: (if Let's Encrypt cert-manager fails)

Plain vars (in hosts.yml)
  ├─ github_app_id ─────────► k8s Secret: github-app (dp-system ns)
  ├─ temporal_address ──────► k8s Secret: temporal-worker-config (dp-system ns)
  └─ temporal_namespace ────► k8s Secret: temporal-worker-config (dp-system ns)
```

---

## 4. Values from current .env

Map from `go-backend/.env` to Ansible variables:

| .env key | Ansible variable | Vault? | Notes |
|----------|-----------------|--------|-------|
| `GITHUBAPP_APPID` | `github_app_id` | No | Use prod value: `2764764` |
| `GITHUBAPP_PRIVATEKEY` | `github_app_private_key` | Yes | RSA private key |
| `CLOUDFLARE_APITOKEN` | `cloudflare_api_token` | Yes | |
| _(not in .env)_ | `temporal_address` | No | From Temporal Cloud dashboard |
| _(not in .env)_ | `temporal_namespace` | No | From Temporal Cloud dashboard |
| _(not in .env)_ | `temporal_cloud_api_key` | Yes | Temporal Cloud → API Keys |
| _(not in .env)_ | `loki_basic_auth_users` | Yes | Generate new with `htpasswd` |

The remaining .env values (`GITHUB_CLIENTID`, `GITHUB_CLIENTSECRET`, `AUTH_*`, `GITEA_*`, `DB_URL`, etc.) are for the Go backend, not the k3s cluster.

---

## 5. Checklist

```
[ ] 1. Create .vault_pass with a strong password
[ ] 2. Add vault_password_file to ansible.cfg
[ ] 3. Add .vault_pass to .gitignore
[ ] 4. ansible-vault create inventory/group_vars/all/secrets.yml
[ ] 5. Fill vault with cloudflare_api_token from .env
[ ] 6. Fill vault with github_app_private_key from .env (prod key)
[ ] 7. Get temporal_address + temporal_namespace from Temporal Cloud, add to hosts.yml
[ ] 8. Create Temporal Cloud API key, add temporal_cloud_api_key to vault
[ ] 9. Generate loki bcrypt hash: htpasswd -nbBC 10 deploy <password>, add to vault
[ ] 10. Add secret creation tasks to k8s_addons role
[ ] 11. Remove empty cloudflare_api_token and loki_basic_auth_users from hosts.yml
[ ] 12. Test: ansible-vault view inventory/group_vars/all/secrets.yml
[ ] 13. Test: ansible-playbook playbooks/site.yml --check
```
