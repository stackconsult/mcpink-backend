## TODO

## Infra

- gVisor and hardening
- [x] Firewalls
- [x] Self hosted grafana for logs and metrics
- [x] Cloudflare DNS
- [x] Ingress contract docs: MCP on `https://mcp.ml.ink/mcp`, webhooks on `https://api.ml.ink`, Cloudflare LB as source of truth

## Backend

- Provision SQLite
- Provision Postgres
- [x] MCP tool resources
- [x] Propagate errors to users

## Product

- [x] Build logs
- [x] Run logs
- Metrics
- Status to user on deployment
- Onboarding
- UI for all MCP clients
- Consider SKILL.md with API

## k3s

- For namespace let's use `userid-projectref`, not `github-projectref`. What if users change their github/gitea.
- Make `userid` short UUID
- Track-only: enforce pod-level `runAsNonRoot` + `allowPrivilegeEscalation=false` for tenant template (`infra/k8s/customer-service-template.yml`) after compatibility validation.
