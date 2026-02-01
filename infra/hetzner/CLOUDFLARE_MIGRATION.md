# Cloudflare Migration Guide

Migrate from direct IP access to Cloudflare DNS for unified domains like `user-app.mcpdeploy.com`.

## Current State

- Muscle servers accessed via IP or `s1.mcpdeploy.com`, `s2.mcpdeploy.com`
- Direct exposure of Hetzner IPs
- No DDoS protection

## Target State

- All apps accessible via `*.mcpdeploy.com` (or your domain)
- Origin IPs hidden behind Cloudflare
- Basic DDoS protection
- WebSocket support
- Optional: Exposed databases on custom ports

---

## What Cloudflare Proxies (Free Tier)

| Traffic Type | Proxied? | Notes |
|--------------|----------|-------|
| HTTP (80) | ✅ Yes | Hidden IP, DDoS protection |
| HTTPS (443) | ✅ Yes | Hidden IP, DDoS protection |
| WebSockets | ✅ Yes | Works over HTTP/HTTPS |
| TCP (Postgres, Redis, etc.) | ❌ No | Requires Spectrum (paid) or DNS-only |

---

## Step 1: Add Domain to Cloudflare

1. Sign up at [cloudflare.com](https://cloudflare.com)
2. Add your domain (e.g., `mcpdeploy.com`)
3. Cloudflare will scan existing DNS records
4. Update nameservers at your registrar to Cloudflare's

```
ns1.cloudflare.com
ns2.cloudflare.com
```

Wait for propagation (up to 24h, usually faster).

---

## Step 2: Configure DNS Records

### For Web Apps (Proxied - Orange Cloud)

```
Type  Name              Content           Proxy
A     app1              157.90.130.187    ☁️ Proxied
A     app2              157.90.130.187    ☁️ Proxied
A     *.apps            157.90.130.187    ☁️ Proxied (wildcard)
```

Proxied records:
- Hide origin IP
- Get DDoS protection
- WebSockets work automatically

### For Databases (DNS Only - Grey Cloud)

If users need to expose databases publicly:

```
Type  Name              Content           Proxy
A     db-userapp        157.90.130.187    DNS only (grey)
```

**Warning:** DNS-only exposes the real IP. Anyone can find it.

### Alternative: Cloudflare Spectrum (Paid)

Spectrum can proxy TCP traffic (Postgres, Redis, custom ports):
- Hides origin IP for TCP
- DDoS protection for TCP
- Pricing: ~$1/GB or enterprise plans

---

## Step 3: Configure SSL/TLS

### In Cloudflare Dashboard

Go to **SSL/TLS** → **Overview**:

| Mode | When to Use |
|------|-------------|
| **Full (Strict)** | ✅ Recommended - Traefik has valid Let's Encrypt cert |
| Full | Traefik has self-signed cert |
| Flexible | No HTTPS on origin (not recommended) |

Set to **Full (Strict)**.

### Edge Certificates

Go to **SSL/TLS** → **Edge Certificates**:

- **Always Use HTTPS**: ON
- **Automatic HTTPS Rewrites**: ON
- **Minimum TLS Version**: 1.2

---

## Step 4: Enable WebSocket Support

WebSockets are enabled by default on Cloudflare free tier.

Verify at **Network** → **WebSockets**: ON

No changes needed to your apps - WebSocket connections over `wss://` just work.

---

## Step 5: Configure Traefik for Real Visitor IPs

When traffic comes through Cloudflare, Traefik sees Cloudflare's IP, not the visitor's.

### Get Real IP from Headers

Cloudflare adds these headers:
- `CF-Connecting-IP` - Visitor's real IP
- `X-Forwarded-For` - IP chain

### Update Traefik Configuration

Add trusted IPs to Traefik's entrypoints. Create/update the dynamic config:

```yaml
# /traefik/dynamic/cloudflare.yaml
http:
  middlewares:
    cloudflare-real-ip:
      headers:
        customRequestHeaders:
          X-Real-IP: "{{ .Request.Header.Get \"CF-Connecting-IP\" }}"
```

Or configure entrypoints to trust Cloudflare IPs (in static config/CLI):

```yaml
entryPoints:
  https:
    address: ":443"
    forwardedHeaders:
      trustedIPs:
        # Cloudflare IPv4
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
        # Cloudflare IPv6
        - 2400:cb00::/32
        - 2606:4700::/32
        - 2803:f800::/32
        - 2405:b500::/32
        - 2405:8100::/32
        - 2a06:98c0::/29
        - 2c0f:f248::/32
```

**Note:** Cloudflare IPs can change. Get current list: https://www.cloudflare.com/ips/

---

## Step 6: (Optional) Lock Down Origin Firewall

For maximum security, only allow HTTP/HTTPS from Cloudflare IPs.

### Hetzner Robot Firewall - Incoming Rules

```
# Priority order (top to bottom)

# SSH - restricted
Name: SSH from Factory    Protocol: tcp    Source: 46.225.65.56/32    Port: 22    Action: accept
Name: SSH from Home       Protocol: tcp    Source: YOUR_IP/32         Port: 22    Action: accept

# HTTP/HTTPS - Cloudflare only
Name: CF 173.245.48.0     Protocol: tcp    Source: 173.245.48.0/20    Port: 80,443    Action: accept
Name: CF 103.21.244.0     Protocol: tcp    Source: 103.21.244.0/22    Port: 80,443    Action: accept
Name: CF 103.22.200.0     Protocol: tcp    Source: 103.22.200.0/22    Port: 80,443    Action: accept
Name: CF 103.31.4.0       Protocol: tcp    Source: 103.31.4.0/22      Port: 80,443    Action: accept
Name: CF 141.101.64.0     Protocol: tcp    Source: 141.101.64.0/18    Port: 80,443    Action: accept
Name: CF 108.162.192.0    Protocol: tcp    Source: 108.162.192.0/18   Port: 80,443    Action: accept
Name: CF 190.93.240.0     Protocol: tcp    Source: 190.93.240.0/20    Port: 80,443    Action: accept
Name: CF 188.114.96.0     Protocol: tcp    Source: 188.114.96.0/20    Port: 80,443    Action: accept
Name: CF 197.234.240.0    Protocol: tcp    Source: 197.234.240.0/22   Port: 80,443    Action: accept
Name: CF 198.41.128.0     Protocol: tcp    Source: 198.41.128.0/17    Port: 80,443    Action: accept
Name: CF 162.158.0.0      Protocol: tcp    Source: 162.158.0.0/15     Port: 80,443    Action: accept
Name: CF 104.16.0.0       Protocol: tcp    Source: 104.16.0.0/13      Port: 80,443    Action: accept
Name: CF 104.24.0.0       Protocol: tcp    Source: 104.24.0.0/14      Port: 80,443    Action: accept
Name: CF 172.64.0.0       Protocol: tcp    Source: 172.64.0.0/13      Port: 80,443    Action: accept
Name: CF 131.0.72.0       Protocol: tcp    Source: 131.0.72.0/22      Port: 80,443    Action: accept

# Default: deny (implicit)
```

**If allowing exposed databases:** Add rules for database ports (wide open or restricted):
```
Name: Postgres            Protocol: tcp    Source: 0.0.0.0/0          Port: 5432-5500    Action: accept
Name: MySQL               Protocol: tcp    Source: 0.0.0.0/0          Port: 3306-3400    Action: accept
Name: Redis               Protocol: tcp    Source: 0.0.0.0/0          Port: 6379-6400    Action: accept
```

---

## Exposing Databases Publicly

Since Cloudflare free tier doesn't proxy TCP, you have options:

### Option A: DNS-Only Subdomain (Free, Exposes IP)

```
db.user-app.mcpdeploy.com ──► 157.90.130.187:5432 (direct, no proxy)
```

Setup:
1. Create DNS-only (grey cloud) A record pointing to muscle IP
2. Expose port on container
3. Open port in Hetzner firewall

**Downside:** IP is exposed for that subdomain.

### Option B: Cloudflare Spectrum (Paid, Hides IP)

Spectrum proxies TCP traffic:
- SSH, Postgres, MySQL, Redis, etc.
- Origin IP hidden
- DDoS protection

Pricing: Enterprise or ~$1/GB on Pro plan

### Option C: Randomized Ports (Obscurity)

Assign random high ports to each database:
```
user-a-postgres: 157.90.130.187:54321
user-b-postgres: 157.90.130.187:54322
```

Combined with firewall rate limiting. Still exposes IP but harder to scan.

---

## Cloudflare Features to Enable

### Free Tier Recommendations

| Setting | Location | Value |
|---------|----------|-------|
| SSL Mode | SSL/TLS → Overview | Full (Strict) |
| Always HTTPS | SSL/TLS → Edge Certificates | ON |
| Min TLS | SSL/TLS → Edge Certificates | 1.2 |
| WebSockets | Network | ON (default) |
| Brotli | Speed → Optimization | ON |
| Auto Minify | Speed → Optimization | JS, CSS, HTML |
| Browser Cache TTL | Caching → Configuration | Respect headers |

### Security Settings

| Setting | Location | Value |
|---------|----------|-------|
| Security Level | Security → Settings | Medium |
| Challenge Passage | Security → Settings | 30 minutes |
| Browser Integrity Check | Security → Settings | ON |
| Bot Fight Mode | Security → Bots | ON (free) |

---

## Verification Checklist

After migration:

- [ ] Domain resolves through Cloudflare (`dig +short yourapp.mcpdeploy.com` shows Cloudflare IPs)
- [ ] HTTPS works (`curl -I https://yourapp.mcpdeploy.com`)
- [ ] WebSockets work (test with a WS client)
- [ ] Traefik logs show real visitor IPs (not Cloudflare IPs)
- [ ] Direct IP access blocked (if firewall locked down)
- [ ] Databases accessible if exposed (via DNS-only subdomain)

---

## Troubleshooting

| Issue | Cause | Fix |
|-------|-------|-----|
| 522 Connection timed out | Cloudflare can't reach origin | Check firewall allows Cloudflare IPs |
| 521 Web server is down | Origin not responding | Check Traefik/Docker is running |
| 525 SSL handshake failed | SSL mode mismatch | Set SSL to "Full" or "Full (Strict)" |
| 526 Invalid SSL certificate | Origin cert invalid | Ensure Let's Encrypt cert is valid |
| WebSocket disconnects | Timeout | Increase Cloudflare timeout or send keepalives |
| Wrong visitor IP in logs | Traefik not reading CF headers | Add trusted IPs config to Traefik |

---

## Cost Summary

| Feature | Cost |
|---------|------|
| DNS + Proxy + DDoS (HTTP/S) | Free |
| WebSockets | Free |
| Basic WAF | Free |
| Spectrum (TCP proxy) | ~$1/GB or Enterprise |
| Advanced DDoS | Pro ($20/mo) or higher |

For most use cases, **free tier is sufficient**.
