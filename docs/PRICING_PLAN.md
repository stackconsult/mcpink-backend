# Pricing Plan

> Last updated: 2026-02-13

---

## Resource Pricing (Usage-Based)

| Resource      | Unit Price                           | Billing    |
| ------------- | ------------------------------------ | ---------- |
| **vCPU**      | $12/vCPU/month                       | Per-second |
| **RAM**       | $6/GB/month                          | Per-second |
| **Disk**      | $0.15/GB/month                       | Monthly    |
| **SQLite DB** | 1 free per service, $2/mo additional | Monthly    |

All compute is billed per-second. You only pay for what you use.

---

## Plans

|                      | Free             | Starter      | Pro           |
| -------------------- | ---------------- | ------------ | ------------- |
| **Price**            | $0/mo            | $5/mo        | $20/mo        |
| **Usage credit**     | —                | $5 included  | $20 included  |
| **Services**         | 1                | 5            | Unlimited     |
| **Max per service**  | 0.5 vCPU, 512 MB | 2 vCPU, 4 GB | 8 vCPU, 32 GB |
| **Custom domains**   | No               | Yes          | Yes           |
| **SQLite databases** | 1                | 5            | Unlimited     |
| **Auto-deploy**      | Yes              | Yes          | Yes           |
| **Build timeout**    | 5 min            | 15 min       | 30 min        |

The plan fee **is** the usage credit. A Starter user consuming $3/mo in resources pays $5 total, not $5 + $3.

Discounts managed via Stripe:

- Annual billing: 20% off plan fee
- Volume discounts: negotiated for Pro users spending >$100/mo

---

## Example Monthly Bills

**Indie dev — small Next.js app + SQLite:**

- 0.5 vCPU ($6) + 1 GB RAM ($6) + 1 GB disk ($0.15) + 1 DB (free) = **$12.15 usage**
- Starter plan: $12.15 - $5 credit = **$12.15 total** ($5 plan covers first $5)

**Startup — API + frontend + DB:**

- API: 1 vCPU ($12) + 2 GB RAM ($12) = $24
- Frontend: 0.5 vCPU ($6) + 512 MB ($3) = $9
- 2 GB disk ($0.30) + 1 DB (free)
- Total usage: **$33.30**
- Pro plan: $33.30 - $20 credit = **$33.30 total**

**Agency — 10 client apps:**

- 10x (0.25 vCPU + 512 MB) = 2.5 vCPU ($30) + 5 GB RAM ($30) + 5 GB disk ($0.75)
- Total usage: **$60.75**
- Pro plan: $60.75 - $20 credit = **$60.75 total**

---

## Cost Basis & Margin Math

### Infrastructure (Hetzner, eu-central-1)

| Server            | Role                        | Specs                                                    | Monthly Cost         |
| ----------------- | --------------------------- | -------------------------------------------------------- | -------------------- |
| ctrl-1 (ctrl)     | K3s control plane, Temporal | Cloud VPS, 8 GB                                          | ~6 EUR               |
| ops-1 (ops)       | git-server, monitoring      | Dedicated: NVMe RAID1                                    | ~37 EUR              |
| build-1 (build)   | BuildKit, Registry          | Dedicated: NVMe RAID1                                    | ~35 EUR              |
| run-1 (run)       | Customer workloads          | Dedicated: EPYC 7502P, 32C/64T, 448 GB ECC, 1x960GB NVMe | ~110 EUR             |
| **Total Hetzner** |                             |                                                          | **~188 EUR (~$208)** |

Additional platform costs (Railway for product server, Temporal, Supabase, Cloudflare, Turso): ~$50–100/mo.

**Total platform cost: ~$270–320/mo.**

### Run Node Unit Economics

The run-1 node is the revenue-generating asset. Everything else is fixed overhead.

**Raw cost per unit on run-1 ($122/mo for 64 vCPU, 448 GB RAM):**

| Resource  | Calculation                | Raw Cost     |
| --------- | -------------------------- | ------------ |
| 1 vCPU    | $122 / 64 threads          | **$1.91/mo** |
| 1 GB RAM  | $122 / 448 GB              | **$0.27/mo** |
| 1 GB Disk | ~$0.03/mo (NVMe amortized) | **$0.03/mo** |

**Sellable capacity** (after ~10 GB RAM + ~4 vCPU for Traefik, gVisor, kubelet):

| Resource | Total  | Overhead | Sellable   |
| -------- | ------ | -------- | ---------- |
| vCPU     | 64     | ~4       | **60**     |
| RAM      | 448 GB | ~10 GB   | **438 GB** |

### Markup at Our Prices

| Resource  | Raw Cost | Our Price | Markup    |
| --------- | -------- | --------- | --------- |
| 1 vCPU    | $1.91    | $12       | **6.3x**  |
| 1 GB RAM  | $0.27    | $6        | **22.2x** |
| 1 GB Disk | $0.03    | $0.15     | **5x**    |

### How We Chose These Numbers

**vCPU at $12/mo:**

- Railway: $20. Render: $14–56. Fly.io dedicated: ~$32. Fly.io shared: ~$2–3.
- $12 is 40% below Railway (the main competitor for agent-deployed apps).
- At 6.3x markup over Hetzner cost, margins are healthy even at low utilization.
- Significantly below Fly.io dedicated ($32) and Render mid-tier ($25+).
- Well above Fly.io shared ($2–3), but we offer dedicated CPU + gVisor isolation.

**RAM at $6/mo:**

- Railway: $10. Render: $12–22. Fly.io extra RAM: ~$5.
- $6 is 40% below Railway, slightly above Fly.io's per-GB add-on rate.
- At 22.2x markup, RAM is our highest-margin resource.
- The EPYC 7502P has 448 GB ECC — we're memory-rich, so aggressive RAM pricing attracts workloads that would otherwise pick Fly.io.

**Disk at $0.15/GB/mo:**

- Railway: $0.16. Fly.io: $0.15. Render: $0.25.
- $0.15 matches Fly.io (cheapest) and undercuts Render by 40%.
- Disk margin is lower (5x) but disk is rarely the revenue driver.

**SQLite at $2/additional DB:**

- Turso Scaler plan ($25/mo) covers 2,500 active databases = ~$0.01/DB.
- Even on Developer ($5/mo), overage is $0.20/DB beyond 500.
- $2/DB is a 10–200x markup depending on our Turso plan, but the value prop is convenience (auto-provisioned, auto-wired env vars).
- 1 free DB per service is a strong differentiator — most competitors charge separately for databases.

### Break-Even Analysis

Typical small container: 0.25 vCPU + 512 MB = $3 + $3 = **$6/mo revenue**.

| Utilization | Containers | Monthly Revenue | Total Cost | Gross Margin        |
| ----------- | ---------- | --------------- | ---------- | ------------------- |
| 5%          | 12         | $72             | $320       | -77%                |
| 10%         | 24         | $144            | $320       | -55%                |
| **~22%**    | **53**     | **$320**        | **$320**   | **0% (break-even)** |
| 25%         | 60         | $360            | $320       | +12%                |
| 50%         | 120        | $720            | $320       | +56%                |
| 75%         | 180        | $1,080          | $320       | +70%                |
| 100%        | 240        | $1,440          | $320       | +78%                |

**Break-even: ~53 small containers (~22% utilization).**

At full utilization of one run node: **~$1,440/mo revenue on $320/mo cost = 78% gross margin.**

### Scaling Economics

Adding a second run node (another Hetzner auction EPYC):

- Incremental cost: ~$122/mo
- Incremental sellable capacity: +60 vCPU, +246 GB RAM
- Break-even for second node: ~20 additional small containers ($122 / $6 per container)
- Overhead stays flat — ctrl, ops, build nodes don't change

Each additional run node is pure leverage.

---

## Competitor Comparison

|                  | Deploy MCP        | Railway           | Fly.io (dedicated) | Render                |
| ---------------- | ----------------- | ----------------- | ------------------ | --------------------- |
| **vCPU/mo**      | **$12**           | $20               | ~$32               | $14–56                |
| **GB RAM/mo**    | **$6**            | $10               | ~$5                | $12–22                |
| **GB Disk/mo**   | **$0.15**         | $0.16             | $0.15              | $0.25                 |
| **Free tier**    | 1 service         | $5 trial credit   | Legacy only        | 0.1 vCPU (spins down) |
| **Platform fee** | $5–20 (is credit) | $5–20 (is credit) | None               | None                  |
| **Billing**      | Per-second        | Per-second        | Per-second         | Fixed tier            |
| **1 free DB**    | Yes               | No                | No                 | No                    |
| **Agent-first**  | Yes (MCP native)  | MCP available     | No                 | No                    |

**Our positioning: 40% cheaper than Railway, agent-native, one-command deploy with auto-wired database.**

---

## Stripe Implementation Notes

- Plans (Free/Starter/Pro) as Stripe Products with monthly/annual Price objects
- Annual prices: 20% discount (e.g., Starter = $48/year instead of $60)
- Usage-based compute: Stripe metered billing with per-second reporting
- Disk and databases: flat monthly line items
- Usage credits: applied as Stripe invoice credits or coupon logic
- Volume discounts for Pro: Stripe Coupons per customer
