## CRITICAL EXECUTION DIRECTIVE (READ FIRST)

Manual-first workflow is mandatory for speed and reliability during migration:

1. Install/test/debug directly on live nodes first to identify what actually works and what fails.
2. Only after behavior is validated manually, codify the exact working steps in Ansible.
3. Use maximum parallelization across nodes/tasks whenever safe.

Do not start with broad full-cluster Ansible changes before manual validation of the specific change path.

# K3s Migration Log

Last updated: 2026-02-10 04:00 UTC

## Goal
Implement `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k3s/ARCHITECTURE_PLAN.md` with reproducible Ansible + Kubernetes manifests, while migrating safely from existing Coolify hosts.
Keep decisions aligned with `/Users/wins/Projects/personal/mcpdeploy/backend/README.md` principles: strict control/build/run separation, shared private registry, and workflow-driven multi-tenant deploys.

## Current Status
- Control plane is up on `k3s-1`.
- Current node health:
  - `k3s-1`, `build-1`, `ops-1`, `run-1`: `Ready`
- All addon Helm releases are currently `deployed` (`cert-manager`, `traefik`, `loki`, `promtail`, `prometheus`, `grafana`).
- `buildkitd` is now scheduling/running on `build-1` after resource tuning.
- `loki` single-binary pod is `Running` on `ops-1`.
- Grafana pod is `Running` on `ops-1`.
- `temporal-worker` image is now published to internal registry and deployment is `Running` on `k3s-1`.
- Vault decryption is now working with `.vault_pass`, and full-cluster `site.yml` apply succeeds.
- Non-secret worker vars are now set in inventory:
  - `github_app_id: 2782695`
  - `temporal_address: us-central1.gcp.api.temporal.io:7233`
  - `temporal_namespace: mlink.svtnk`
- Current secret state in `dp-system`:
  - present: `github-app`, `temporal-worker-config`, `temporal-creds`, `loki-auth-secret`
- Grafana ingress now routes only `grafana.ml.ink` (removed `ops.ml.ink` alias by request).
- Traefik is now running on `run-1` (`pool=run`) per architecture separation.
- `run-1` legacy Coolify containers/directories have been removed and Docker is masked to prevent port conflicts on `80/443`.
- Prometheus `node-exporter` targets are `up=1` on all four nodes (`10.0.0.3`, `10.0.0.4`, `10.0.1.3`, `10.0.1.4`).
- `dp-system` ingresses are now reconciled to `ml.ink`:
  - `grafana.ml.ink`
  - `loki.ml.ink`
- Wildcard certificate is healthy: `wildcard-apps` `Ready=True`, secret `wildcard-apps-tls` present.
- Direct origin checks on run node are healthy:
  - `grafana.ml.ink` returns `302` (`/login`)
  - `loki.ml.ink/ready` returns `401` (expected due Traefik basic auth)
- **Cloudflare Load Balancing** is active on `*.ml.ink`:
  - LB hostname: `*.ml.ink` → origin pool `run-nodes` → `157.90.130.187` (run-1)
  - HTTPS health check on port 443, interval 60s
  - SSL mode: Full (Cloudflare terminates edge TLS, connects to origin via HTTPS)
  - Wildcard cert `*.ml.ink` issued by Let's Encrypt via cert-manager (auto-renews every 90 days)
  - `grafana.ml.ink` returns 302 (healthy), `loki.ml.ink` returns 401 (expected, basic-auth protected)
  - Adding run nodes = add IP to Cloudflare origin pool; LB auto-routes and health-checks
- Docker CE fully purged from `run-1` and `build-1` (packages, `/var/lib/docker`, `docker0` bridge, apt source removed).
- Internal registry currently contains `dp-system/temporal-worker` repository.
- Gitea deployment manifest and ingress added (`gitea.yml`, `gitea-ingress.yml`) — pending first apply.
- Gitea is HTTPS-only (SSH disabled); all traffic via CF LB → Traefik.

## Completed
- [x] Scaffolded Ansible roles/playbooks and Kubernetes manifests in:
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible`
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s`
- [x] Corrected stale registry IP in `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s/temporal-worker.yml` (`10.0.1.4`)
- [x] Fixed `ops-1` join failure by using reachable server endpoint (`k3s_server_endpoint_ip: 10.0.0.4`) plus dedicated-server routing
- [x] Fixed Helm connectivity by setting `KUBECONFIG=/etc/rancher/k3s/k3s.yaml` in Ansible `k8s_addons` role
- [x] Fixed Traefik chart schema mismatch in `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s/traefik-values.yml`
- [x] Removed stale static GitHub installation ID wiring:
  - removed `GITHUB_INSTALLATION_ID` from `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s/temporal-worker.yml`
  - removed static `installation-id` from the old manual secret-manifest flow
  - updated `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/MIGRATION_VAULT.md` to rely on per-workflow installation ID from backend DB
- [x] Added reproducible secret bootstrap tasks in `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/roles/k8s_addons/tasks/main.yml`:
  - `github-app` secret (`app-id`, `private-key`)
  - `temporal-creds` secret (`cloud-api-key`)
  - `temporal-worker-config` secret (`temporal-address`, `temporal-namespace`)
  - all gated by vars presence to keep reruns safe/idempotent
- [x] Aligned Temporal worker auth with actual backend config usage:
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s/temporal-worker.yml` now uses `TEMPORAL_CLOUDAPIKEY` from `temporal-creds.cloud-api-key`
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/roles/k8s_addons/tasks/main.yml` now creates `temporal-creds` with `cloud-api-key` from `temporal_cloud_api_key`
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k3s/ARCHITECTURE_PLAN.md` updated to same key/env contract
- [x] Added k8s worker runtime command in backend (wired like main worker):
  - `/Users/wins/Projects/personal/mcpdeploy/backend/go-backend/cmd/k8s-worker/main.go`
  - `/Users/wins/Projects/personal/mcpdeploy/backend/go-backend/k8s-worker.Dockerfile`
  - `run-k8s-worker` target in `/Users/wins/Projects/personal/mcpdeploy/backend/go-backend/Makefile`
  - registers existing account/deployments workflows + activities and starts Temporal worker lifecycle
- [x] Added vault password file setting in `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/ansible.cfg` (`vault_password_file = .vault_pass`)
- [x] Fixed vault + inventory loading blockers:
  - rekeyed local vault password file to current vault pass
  - converted `inventory/group_vars/all/secrets.yml` from dotenv format to valid YAML (still vaulted)
  - moved shared defaults from `inventory/group_vars/all.yml` to `inventory/group_vars/all/main.yml` because `inventory/group_vars/all/` exists (directory/file name collision)
- [x] Validation checks for this pass:
  - `go test ./cmd/...` passed (includes new `cmd/k8s-worker`)
  - `k3s kubectl apply --dry-run=client` passed for updated `temporal-worker.yml`
  - secret bootstrap path is Ansible-managed via `roles/k8s_addons` (no manual `infra/k8s/*.example.yml` applies)
- [x] Removed orphan manual secret examples from `infra/k8s/`:
  - `cloudflare-api-token-secret.example.yml`
  - `github-app.example.yml`
  - `loki-auth-secret.example.yml`
  - `temporal-creds.example.yml`
  - `temporal-worker-config.example.yml`
- [x] Tuned workloads for current node capacity:
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s/buildkit.yml` resources reduced to schedule on 16Gi build node
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s/loki-values.yml` disabled `chunksCache` and `resultsCache` defaults
- [x] Re-applied controller pipeline with Homebrew Ansible:
  - `ansible-playbook -i inventory/hosts.yml playbooks/site.yml --limit k3s-1`
  - result: `ok=43 changed=11 failed=0`
- [x] Re-applied full cluster pipeline:
  - `ansible-playbook -i inventory/hosts.yml playbooks/site.yml`
  - result:
    - `k3s-1: ok=46 changed=12 failed=0`
    - `build-1: ok=28 changed=1 failed=0`
    - `ops-1: ok=34 changed=0 failed=0`
    - `run-1: ok=39 changed=25 failed=0`
  - `run-1` is now joined and `Ready`
- [x] Set non-secret Temporal/GitHub worker vars in inventory and reconciled:
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/inventory/hosts.yml`
  - `github_app_id: 2782695`
  - `temporal_address: us-central1.gcp.api.temporal.io:7233`
  - `temporal_namespace: mlink.svtnk`
  - `ansible-playbook -i inventory/hosts.yml playbooks/site.yml --limit k3s-1`
  - result: `k3s-1: ok=50 changed=16 failed=0`
- [x] Verified worker secrets now exist in cluster:
  - `github-app`, `temporal-worker-config`, `temporal-creds`, `loki-auth-secret`
- [x] Applied Grafana PVC permission fix and verified live pod:
  - set `initChownData.enabled: false` in `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s/grafana-values.yml`
  - reconciled via `site.yml`; current Grafana pod is `Running`
- [x] Added Grafana ingress host for requested domain:
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s/grafana-ingress.yml` routes:
    - `grafana.ml.ink`
- [x] Removed `ops.ml.ink` mapping to Grafana ingress by request:
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s/grafana-ingress.yml` no longer contains `ops.ml.ink`
  - direct origin check now returns `404` for `Host: ops.ml.ink` on Traefik
- [x] Fixed containerd HTTP registry resolution in practice:
  - added Ansible-managed overrides for `/var/lib/rancher/k3s/agent/etc/containerd/certs.d/*/hosts.toml`
  - verified `k3s crictl pull registry.internal:5000/...` now returns normal registry responses (no HTTPS protocol mismatch)
- [x] Built + published temporal worker image and unblocked rollout:
  - built `linux/amd64` image from `/Users/wins/Projects/personal/mcpdeploy/backend/go-backend/k8s-worker.Dockerfile`
  - pushed `registry.internal:5000/dp-system/temporal-worker:latest`
  - verified `temporal-worker` pod is `Running` and connected to Temporal Cloud
- [x] Recovered observability scheduling after stale local-path node affinity:
  - destructively recreated Grafana/Loki PVC/PV bindings on current `ops-1` node
  - verified Grafana + Loki pods are `Running`
- [x] Added Grafana datasource provisioning:
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s/grafana-values.yml`
  - default Prometheus + Loki datasources now provisioned by Helm values
- [x] Removed registry rolling-update overlap issue:
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s/registry.yml` now uses `strategy.type: Recreate`
  - avoids duplicate pending registry pod during rollouts
- [x] Improved registry mirror template for insecure internal registry:
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/roles/registry_client/templates/registries.yaml.j2`
  - added explicit HTTP mirror endpoints + TLS skip-verify stanzas
- [x] Applied cluster-wide k3s registry fallback setting:
  - `k3s_disable_default_registry_endpoint: true`
  - rendered in both server/agent configs
  - `ansible-playbook -i inventory/hosts.yml playbooks/site.yml`
  - result:
    - `k3s-1: ok=51 changed=18 failed=0`
    - `build-1: ok=29 changed=3 failed=0`
    - `ops-1: ok=35 changed=3 failed=0`
    - `run-1: ok=34 changed=4 failed=0`
- [x] Recovered BuildKit from lock-related crashloop:
  - updated `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s/buildkit.yml` with:
    - `strategy.type: Recreate` (avoids overlap between old/new pods)
    - init container `clear-stale-lock` removing `/var/lib/buildkit/buildkitd.lock`
  - reconciled via `ansible-playbook -i inventory/hosts.yml playbooks/site.yml --limit k3s-1`
  - verified live pod `buildkitd-6f464c5f9f-*` is `Running` on `build-1`
- [x] Added explicit OS patching workflow:
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/playbooks/patch-hosts.yml`
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/roles/security_patch`
- [x] Cleaned config hygiene:
  - removed duplicate `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/group_vars/`
  - standardized on `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/inventory/group_vars/`
  - renamed ambiguous `private_network_cidr` -> `vswitch_network_cidr`
  - added `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/inventory/group_vars/ctrl.yml`
- [x] Recovered `run-1` rejoin failure (`Node password rejected`) and restored node readiness:
  - deleted stale `muscle-1.node-password.k3s` secret on control-plane
  - restarted `k3s-agent` on `run-1` after agent state cleanup
  - verified `muscle-1` `Ready`
- [x] Switched Traefik back to run pool in `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s/traefik-values.yml` (`pool: run`)
- [x] Added reproducible legacy cleanup to Ansible:
  - new role `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/roles/legacy_cleanup`
  - wired in `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/playbooks/site.yml`
  - enabled for run nodes in `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/inventory/group_vars/run.yml`
  - removes Coolify containers/files, flushes stale Docker DNAT rules, masks Docker service/socket
- [x] Added run-node Traefik ingress/probe firewall policy in `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/roles/firewall/tasks/main.yml`
  - allow public `80/443`
  - allow private `8080/19100` from cluster CIDRs
- [x] Verified all nodes publish metrics:
  - Prometheus `up{job="node-exporter"}` shows all node targets healthy
- [x] Fixed and validated registry GC retention path:
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k8s/registry-gc.yml` now falls back to on-disk repository discovery when `_catalog` fails
  - manual CronJob run succeeded and evaluated repository retention correctly
- [x] Completed domain cutover in infra code/docs from `breacher.org` to `ml.ink`:
  - updated `/Users/wins/Projects/personal/mcpdeploy/backend/infra/k3s/ARCHITECTURE_PLAN.md`
  - updated `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/README.md`
  - confirmed manifests remain `grafana.ml.ink`/`loki.ml.ink`
- [x] Reconciled control plane with domain updates:
  - `ansible-playbook -i inventory/hosts.yml playbooks/site.yml --limit k3s-1`
  - result: `k3s-1: ok=53 changed=17 failed=0`
  - cluster now serves `grafana.ml.ink` + `loki.ml.ink` on ingresses
- [x] Added reproducible Cloudflare LB playbook:
  - `/Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible/playbooks/cloudflare-lb.yml`
  - auto-builds origins from `run` inventory group
  - manages pool + hostnames (`*.ml.ink`, `grafana.ml.ink`, `loki.ml.ink`)
  - includes explicit token scope validation and clear failure message
- [x] Removed final legacy Coolify path residue on run node:
  - deleted `/data/coolify` on `run-1` after cleanup verification
- [x] Purged Docker CE from `run-1` and `build-1`:
  - `apt-get purge` docker-ce, docker-ce-cli, containerd.io, buildx/compose plugins
  - removed `/var/lib/docker`, `/etc/docker`, docker0 bridge interface
  - removed Docker apt source (`/etc/apt/sources.list.d/docker.list`)
  - k3s containerd unaffected (separate runtime)
- [x] Set up Cloudflare Load Balancing for public ingress:
  - created Cloudflare LB on `*.ml.ink` with origin pool `run-nodes` (run-1: 157.90.130.187)
  - HTTPS health monitor on port 443, 60s interval
  - Cloudflare SSL mode set to Full
  - wildcard cert `*.ml.ink` issued via cert-manager/Let's Encrypt, stored as `wildcard-apps-tls`
  - Traefik TLSStore serves cert as default; all ingresses work without per-ingress TLS config
  - verified end-to-end: Cloudflare edge → run-1 → Traefik → Grafana (302), Loki (401)
  - architecture decision: Cloudflare LB ($5/mo) chosen over Hetzner Cloud LB (bandwidth limits) and free CF proxy (no active health checks)

## TODO (Next Actions)
- [x] Re-run addon stage and confirm all Helm releases are `deployed`:
  - `traefik`, `loki`, `promtail`, `prometheus`, `grafana`
- [x] Fix `temporal-worker` image bootstrap and pull path:
  - build + push image: `registry.internal:5000/dp-system/temporal-worker:latest`
  - containerd HTTP registry resolution adjusted and validated
- [x] Wire `go-backend/cmd/k8s-worker` to run real worker registrations (existing account/deployments workflows)
- [x] Fix Grafana rollout pod init permissions (`init-chown-data` CrashLoopBackOff on existing PVC files)
- [x] Confirm `dp-system` manifest health after above fixes:
  - registry, registry-gc, buildkit, temporal-worker, ingresses, runtimeclass `gvisor`
- [x] Recover run pool node and restore architecture placement:
  - `run-1` rejoined and `Ready`
  - Traefik DaemonSet nodeSelector moved back to `pool=run`
- [x] Set remaining worker vars and apply secrets:
  - set `github_app_id`, `temporal_address`, `temporal_namespace`
  - ensure `github_app_private_key` exists in vault
  - re-run `site.yml` and verify `github-app` + `temporal-worker-config` secrets exist
- [x] Verify Promtail readiness after Loki/Grafana stabilization
- [x] Validate registry GC CronJob behavior in-cluster
- [x] Finalize public ingress DNS/LB:
  - Cloudflare Load Balancing active on `*.ml.ink`
  - origin pool `run-nodes` with health checks
  - `grafana.ml.ink` and `loki.ml.ink` served via wildcard LB
- [ ] Deploy Gitea on ops-1:
  - apply `gitea.yml` + `gitea-ingress.yml` via `site.yml` or `--limit k3s-1`
  - `git.ml.ink` already covered by `*.ml.ink` Cloudflare LB (HTTPS-only, no SSH)
  - create admin user, generate API token, add `gitea_admin_token` + `gitea_webhook_secret` to vault
  - update Railway env: `GITEA_BASEURL=https://git.ml.ink`
  - verify: pod running, HTTPS API responds, ingress active
- [ ] Run full verification checklist from architecture plan section 16
- [ ] Run security patch playbook on all nodes in maintenance window
- [ ] Pin mutable infra image tags to immutable versions/digests after runtime validation:
  - `moby/buildkit:latest`
  - `registry.internal:5000/dp-system/temporal-worker:latest`
  - `registry:2` (including `registry-gc` job image)
- [ ] Implement k8s-native worker activities (currently placeholders returning `not implemented`)
- [ ] Switch backend deployment workflow execution from Coolify workflow path to k8s-native workflow path

## Short Learnings (for future self)
### What worked
- Ansible roles are idempotent enough for repeated reruns.
- Joining mixed-network nodes works when dedicated hosts have explicit routes to the cloud subnet and agents use a reachable server endpoint.
- `k3s kubectl` works reliably even when plain `kubectl`/`helm` lacks kubeconfig.
- Backend-driven workflow input is the right source of truth for tenant-specific GitHub installation IDs; cluster-level static env vars are incorrect for multi-tenant.
- Re-running `site.yml` on `k3s-1` is fast and safe for iterative convergence.
- Go compile checks are green for new command wiring (`go test ./cmd/...`).
- Disabling Grafana `initChownData` solved PVC permission crashloops on existing volumes.
- BuildKit rollouts with hostPath state can deadlock on stale lock files unless update strategy avoids overlap and stale lock is cleaned before startup.
- Cleaning run-node legacy proxy state must include both containers and stale Docker DNAT rules, otherwise host ports can appear free but traffic is still hijacked.
- Cloudflare Load Balancing ($5/mo) is the right ingress strategy for bare-metal run nodes: no bandwidth limits (traffic flows through CF edge, not a proxy box), active health checks remove dead origins, adding run nodes = adding an IP to the pool.
- Hetzner Cloud LB has bandwidth caps (~5TB/mo on LB11) and all traffic flows through it — bad for a platform hosting customer apps. Cloudflare LB avoids this.
- Free Cloudflare proxy only does passive retry-on-failure (not active health checks); user-facing latency spikes when an origin is down.

### What didn’t / gotchas
- `ops-1` could not reach `10.0.0.4:6443` over private path; agent looped on `127.0.0.1:6444/cacerts` errors.
- Helm failed with `localhost:8080` until kubeconfig was exported in tasks.
- Traefik chart no longer accepts `ports.web.redirectTo`; must use:
  `ports.web.http.redirections.entryPoint.to: websecure`.
- Chart defaults can over-provision for small nodes (e.g., Loki memcached caches enabled by default, large BuildKit requests); values must be tuned to actual node capacity.
- Grafana chart init container can fail on existing data directories due ownership constraints; rollout may keep one old pod healthy and one new pod crash-looping.
- `temporal-worker` failures currently combine two issues: image not pushed yet and containerd HTTP/HTTPS registry handling noise.
- Even with `k3s_disable_default_registry_endpoint: true`, generated containerd hosts still resolve `registry.internal:5000` via HTTPS first in this setup.
- Traefik `hostNetwork + hostPort 80/443` on pool=run hard-conflicts with any legacy proxy still occupying those ports on run nodes.
- Cloudflare `521` can persist even when cluster and origin host are healthy; verify by:
  - testing origin directly with `Host` header
  - checking whether Cloudflare requests actually hit origin firewall counters
  - validating Cloudflare DNS/origin settings separately from Kubernetes.
- Cloudflare token used for cert-manager DNS automation may not include Load Balancer API scopes; zone lookup can succeed while all LB endpoints still fail with `Authentication error`.
- Traefik TLSStore may cache the absence of a secret; creating the secret alone may not unblock — a `rollout restart daemonset traefik` forces re-read.
- Docker CE on k3s agent nodes is dead weight — k3s uses containerd directly. Purging Docker frees ~400-700MB and eliminates `docker0` bridge confusion.
- `inventory/group_vars/all.yml` is ignored when `inventory/group_vars/all/` directory exists; shared defaults must live in `inventory/group_vars/all/main.yml`.
- Vaulted `group_vars` must decrypt to YAML dict format. Dotenv `KEY=VALUE` content causes variable merge errors (`expected dicts ... _AnsibleTaggedStr`).
- Once encrypted group vars are present, Ansible live apply depends on matching vault password; syntax checks can pass even when runtime decryption fails.
- Long remote `ansible-playbook` runs can appear silent for stretches; always verify progress with node-level `journalctl` and cluster checks.

## Fast Resume Commands
```bash
cd /Users/wins/Projects/personal/mcpdeploy/backend/infra/ansible

# continue control-plane/addon rollout
ansible-playbook -i inventory/hosts.yml playbooks/site.yml

# configure cloudflare load balancer hostnames/pool
ansible-playbook -i inventory/hosts.yml playbooks/cloudflare-lb.yml

# then verify
ssh root@46.225.100.234 'k3s kubectl get nodes -o wide'
ssh root@46.225.100.234 'KUBECONFIG=/etc/rancher/k3s/k3s.yaml helm list -A'
ssh root@46.225.100.234 'k3s kubectl -n dp-system get all,ingress'
ssh root@46.225.100.234 'k3s kubectl -n dp-system get secrets github-app temporal-worker-config temporal-creds'
ssh root@46.225.100.234 'watch -n 3 \"k3s kubectl -n dp-system get pods -o wide\"'
ssh root@46.225.100.234 'k3s kubectl -n dp-system get events --sort-by=.lastTimestamp | tail -n 40'
ssh root@116.202.163.209 'systemctl is-active k3s-agent && journalctl -u k3s-agent -n 40 --no-pager'

# security patching (explicit, separate)
ansible-playbook -i inventory/hosts.yml playbooks/patch-hosts.yml --limit all
```
