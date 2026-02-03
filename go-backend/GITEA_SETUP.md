# Gitea Integration Setup

## Implementation Status: COMPLETE ✓

All code has been implemented. The following configuration is needed to activate the integration.

---

## Required Configuration

| Variable | Description | Status |
|----------|-------------|--------|
| `GITEA_ADMINTOKEN` | Gitea API token with admin privileges | ✅ DONE |
| `GITEA_WEBHOOKSECRET` | Secret for verifying webhook signatures | ⏳ PENDING |
| `GITEA_COOLIFYPRIVATEKEYUUID` | Coolify's UUID for the SSH deploy key | ⏳ PENDING |

---

## Setup Steps

### Step 1: Create Gitea Admin Token ✅

Already done. Token saved to `.env`.

---

### Step 2: Generate Webhook Secret

```bash
openssl rand -hex 32
```

Add to `.env`:
```
GITEA_WEBHOOKSECRET=<generated-secret>
```

---

### Step 3: SSH Deploy Key for Coolify

This allows Coolify to pull code from Gitea via SSH.

#### 3a. Generate SSH Keypair
```bash
ssh-keygen -t ed25519 -C "coolify-gitea-deploy" -f gitea-deploy-key -N ""
```

Creates:
- `gitea-deploy-key` (private key)
- `gitea-deploy-key.pub` (public key)

#### 3b. Add Public Key to Gitea Admin Account
1. Log into `git.ml.ink` as admin
2. **Settings** → **SSH / GPG Keys** → **Add Key**
3. Paste contents of `gitea-deploy-key.pub`
4. Save

#### 3c. Upload Private Key to Coolify
1. Coolify → **Security** → **Private Keys** → **Add**
2. Paste contents of `gitea-deploy-key`
3. Name: `gitea-deploy`
4. Save → Copy the **UUID**

Add to `.env`:
```
GITEA_COOLIFYPRIVATEKEYUUID=<uuid-from-coolify>
```

---

## Final `.env` Config

```bash
GITEA_ENABLED=true
GITEA_BASEURL=https://git.ml.ink
GITEA_ADMINTOKEN=48d83c13d5c33...      # ✅ DONE
GITEA_USERPREFIX=u
GITEA_WEBHOOKSECRET=<step-2>           # ⏳ PENDING
GITEA_SSHURL=git@git.ml.ink
GITEA_COOLIFYPRIVATEKEYUUID=<step-3>   # ⏳ PENDING
```

---

## After Configuration

Restart the server. The following MCP tools will be available:

| Tool | Description |
|------|-------------|
| `create_repo(name)` | Creates repo at `ml.ink/u-{user_id}/{name}` |
| `get_push_token(repo)` | Gets fresh push credentials |
| `create_app(repo="ml.ink/...")` | Deploys from internal git |

Webhook endpoint: `POST /webhooks/internal-git` (auto-redeploy on push)

---

## Files Modified

```
go-backend/
├── application.yaml                          # Added gitea config section
├── sqlc.yaml                                 # Added internalrepos
├── internal/
│   ├── storage/pg/
│   │   ├── migrations/
│   │   │   ├── 0021_internal_repos.sql      # NEW: internal_repos table + gitea_username
│   │   │   └── 0022_apps_git_provider.sql   # NEW: git_provider column
│   │   └── queries/
│   │       ├── internalrepos/internalrepos.sql  # NEW
│   │       ├── users/users.sql              # Added gitea queries
│   │       └── apps/apps.sql                # Added git_provider
│   ├── internalgit/                         # NEW: entire package
│   │   ├── config.go
│   │   ├── types.go
│   │   ├── client.go
│   │   ├── repos.go
│   │   └── service.go
│   ├── coolify/
│   │   └── applications.go                  # Added CreatePrivateDeployKey
│   ├── deployments/
│   │   ├── types.go                         # Added GitProvider fields
│   │   ├── activities.go                    # Added CreateAppFromInternalGit
│   │   ├── workflow.go                      # Branch for gitea vs github
│   │   └── service.go                       # Added RedeployFromInternalGitPush
│   ├── webhooks/
│   │   ├── handlers.go                      # Added gitea config, new route
│   │   └── internalgit.go                   # NEW: webhook handler
│   ├── mcpserver/
│   │   ├── server.go                        # Added internalGitSvc
│   │   ├── types.go                         # Added CreateRepo/GetPushToken types
│   │   ├── tools.go                         # Updated create_app for gitea
│   │   └── tools_repo.go                    # NEW: unified repo tools
│   └── bootstrap/
│       ├── config.go                        # Added Gitea config
│       └── internalgit.go                   # NEW: service provider
└── cmd/server/main.go                       # Added providers
```

---

## Testing Checklist

After configuration:

- [ ] `create_repo(name="test-app")` returns `ml.ink/u-xxx/test-app` + git remote
- [ ] Can push code to the git remote
- [ ] `create_app(repo="ml.ink/u-xxx/test-app", branch="main", name="test")` deploys
- [ ] Push to repo triggers auto-redeploy via webhook
- [ ] GitHub flow still works (regression test)
