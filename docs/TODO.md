## TODO

## Infra

- [x] Firewalls
- [x] Self hosted grafana for logs and metrics
- [x] Cloudflare DNS
- [x] Ingress contract docs: MCP on `https://mcp.ml.ink/mcp`, webhooks on `https://api.ml.ink`, Cloudflare LB as source of truth

## Backend

- Provision SQLite
- Provision Postgres
- [x] MCP tool resources

## Product

- UI for all MCP clients
- Consider SKILL.md with API

## k3s

- For namespace let's use `userid-projectref`, not `github-projectref`. What if users change their github/gitea.
- Make `userid` short UUID
- Track-only: enforce pod-level `runAsNonRoot` + `allowPrivilegeEscalation=false` for tenant template (`infra/k8s/customer-service-template.yml`) after compatibility validation.
- If user deletes service while it's being deployed the deployment service will stop and likely will not clean itself
- make sure all ansible manifests match the infrastructure, nothing should be provisioned manually
- let's not show graphql errors like `errors="input: me failed to get Firebase user: context canceled\n"` it means user refreshed the page before query loaded. 
