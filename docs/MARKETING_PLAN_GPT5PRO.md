# Deploy MCP / ml.ink — Marketing Ideas Playbook

Your product is unusually demo-friendly: it turns *code* into a *public URL* with minimal human involvement. Lean into “proof of autonomy” and make every piece of content show **time-to-URL** and **error-to-green** recovery.

---

## The core story (repeat everywhere)

1. Agents can write apps now.  
2. They still can’t deploy without humans.  
3. Deploy MCP makes deploys agent-native: one tool → one URL → no dashboards.

**Signature format:** *agent sees failure → reads logs → fixes → redeploys → green*.

---

## Video ideas beyond “framework X deploys”

### Constraint videos (sports / rules = shareable)
- “Deploy a full-stack app using only **3 tool calls**.” (tool-call counter on screen)
- “Deploy a SaaS where the agent **can’t use the word ‘Docker’**.”
- “Deploy an app in **90 seconds** before I cut the feed.” (timer + hard stop)
- “Deploy a project where every commit message must be a **haiku**.”
- “Deploy from **one sentence**… then ship a bugfix from **a second sentence**.”

### Error-to-green mini-dramas (highest conversion)
- “Port detection fails → agent reads error → sets port → redeploys.”
- “Missing env var → agent discovers it in logs → provisions resource → auto-wires → redeploys.”
- “Crash loop speedrun: fastest fix wins.” (you vs model vs model)
- “Deliberately broken app deployment… watch the agent self-heal it.”

### Agent-as-SRE series (recurring storyline)
- “Production incident #1: OOM (memory limit) → agent scales + redeploys.”
- “Incident #2: bad deploy → agent rolls back.”
- “Incident #3: DB token rotated → agent re-wires secrets + confirms health.”

### Economics / scale porn (infra crowd + trust)
- “100 deploys: time, cost, failures, retries.” (real stats)
- “Build cache hits: why the 10th deploy is 5× faster.”
- “What rate limits look like when an agent goes feral (and how the platform contains it).”

### Reverse demo (start with the URL)
Open with the live URL, then reveal:
- “This app didn’t exist 60 seconds ago.”
- rewind/timelapse the agent creating → pushing → deploying.

### Recursion ladder (meta, but controlled)
- “Agent deploys an MCP server that deploys other MCP servers.”
- “Agent deploys a bot that deploys bots — with quotas + TTL safety rails.”
- “Clawnbot deploys itself, then spins up a demo farm of 20 mini-apps.”

**Important:** for any “self replication” joke, show the guardrails (quota, TTL, rate limits) as part of the flex.

---

## Weird stunts that can go viral (without being spammy)

### 1) The “URL receipt” share card
After every deploy, generate a shareable card image:
- app name + deployed URL
- time-to-URL
- detected framework/buildpack
- “deployed by an agent” stamp

People share images more than text. Turn every deploy into a tiny billboard.

### 2) Deploy Roulette (curated)
A public page where users submit:
- a public repo link (or pick from a curated list)
- a prompt: “make it deployable”

Your agent attempts it live with a scoreboard:
- ✅ success
- ❌ fail (with reason code + log tail)

### 3) The Museum of Useless Apps
A gallery of tiny, hilarious apps at `*.ml.ink`:
- “rate my houseplant”
- “excuse generator for meetings”
- “a button that apologizes”

Each includes an opt-in “deployed on ml.ink” footer.

### 4) The 1-minute Startup (weekly)
Viewers submit ideas → you pick one → agent builds + deploys in 60–180 seconds → you post the URL.

### 5) The “No Dashboard” pledge
A clear identity move:
- “If it requires clicking around a provider UI, it doesn’t count.”
Then deploy real things without opening dashboards.

---

## Content that earns trust and backlinks

### “Agent-first infra” essay series
Concrete + technical, but readable:
- Why agent deployments need **task semantics** (progress/cancel/resume)
- Create vs retry-safe (idempotency keys vs magical upserts)
- Error messages as UX for non-human users
- Anatomy of an autonomous deploy loop

### Publish reason codes as public docs
Example: `BUILD_FAILED`, `PORT_NOT_LISTENING`, `HEALTHCHECK_TIMEOUT`, etc.

Agents love enums. Developers love clarity.

### “Make your repo agent-deployable” checklist
A guide that’s useful even if they don’t use you (but suggests you as the obvious destination):
- PORT handling
- health endpoint
- env var patterns
- build/runtime commands
- migrations/seed conventions

Bonus: ship a GitHub Action that runs the checklist and comments on PRs.

---

## Product-led marketing loops (built-in distribution)

### “Deploy to ml.ink” button (agent-native)
A README badge that opens a page with:
- copy/paste MCP config for Claude/Cursor/Windsurf
- a prompt snippet that deploys the repo

Each starter repo becomes a distribution channel.

### Preview URLs with TTL by default
Every PR/branch deploy gets:
- `https://repo-pr-42.ml.ink`
- auto-cleanup after 72h

Now every PR comment thread carries your brand.

### Celebrate the “first deploy” moment
Free tier is marketing spend. Make it shareable:
- confetti moment
- share card
- “your first URL is live” story

### Referral by subdomain
Offer extra build minutes / service caps in exchange for keeping an opt-in footer/badge.

---

## Distribution moves beyond “post it everywhere”

### Creator kits
Make a “demo kit” for YouTubers:
- 5 prompts + 5 repos + 5 failure scenarios that recover cleanly
- thumbnails, B-roll, suggested titles
Creators love plug-and-play.

### Deploy clinic (live)
Weekly stream:
- viewers submit repos
- you pick 3
- agent tries to deploy
- you narrate results and lessons

### Micro-partnerships
Ship a starter + a video + a post for pairings:
- Turso + ml.ink
- SvelteKit + ml.ink
- FastAPI + ml.ink

---

## Branding: “ml.ink everywhere” without being annoying

### Petname subdomains
Lean into weird/cute default names (“rename later”). People share weird names.

### “receipt endpoint”
Every service exposes an endpoint like `/.mlink` showing:
- deployed at
- commit SHA
- framework
- status/health
- “deployed by agent” stamp

Screenshots will include your brand naturally.

### Stickers that are useful
QR stickers that go to:
- “Add ml.ink to Claude/Cursor in 30 seconds”

---

## Launch cadence (won’t fry your brain)

### Week 1–2: Build the demo engine
- 10 stack demos (SEO)
- 5 error-to-green loops (conversion)
- 1 speedrun (viral)

### Week 3–4: Build the loops
- share cards
- gallery (opt-in)
- preview TTL

### Month 2: Build authority
- publish reason codes
- publish “agent deployable repo” checklist
- run weekly deploy clinic

---

## Two caution flags

### “Deploy it for me” bot
Safe version:
- explicit opt-in
- strict rate limits
- public repos only
- default TTL previews
- abuse reporting

### “Replicates itself” gimmicks
Make it a joke *with* safety rails:
- quotas
- TTL
- cleanup
- deterministic names
The guardrails become part of the flex.
