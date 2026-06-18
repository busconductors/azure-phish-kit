# Azure Phishing Kit — Fragment-Based PhaaS Architecture

> Replicates the commercial PhaaS attack analyzed from an active phishing campaign.
> Full threat intel: `docs/threat-intel-report.md`

## What This Is

A production-grade phishing kit using the same architecture as commercial
Phishing-as-a-Service platforms. Fragment-based AES-256-GCM encrypted payload
delivery. CDN fronting with Cloudflare Workers. Branded landing pages for
Microsoft 365, Google Workspace, and Okta.

## Architecture (5 layers)

```
KeyGen → Payload Encrypt → URL Assembly → Landing Page Decrypt → Credential Capture
  │            │                │                │                    │
  │    AES-256-GCM      https://host/     Client-side JS     POST /capture
  │    random nonce     ?victim#enc       Web Crypto API     302 → real login
  │    per victim       fragment          decrypts + renders
  │
  └── AES-256 key shared between payload-generator and landing page
```

## Quick Start

```bash
# 1. Generate encryption key
cd payload-generator
go run keygen.go
# Copy the base64 key

# 2. Embed in landing page
# Edit ../landing-page/index.html
# Set: const AES_KEY_B64 = '<your-key>';

# 3. Generate a phishing URL
cd ../scripts
AES_KEY='<your-key>' PHISH_HOST='localhost:9090' ./generate-url.sh \
    --email victim@corp.com --brand microsoft

# 4. Start capture backend
cd ../capture-backend
STORAGE_KEY='<your-key>' go run main.go

# 5. Serve landing page (test)
cd ../landing-page
python3 -m http.server 9090

# 6. Open the generated URL in browser
```

## Components

| Component | Directory | Purpose |
|-----------|-----------|---------|
| Payload Generator | `payload-generator/` | Encrypts lure config with AES-256-GCM |
| Landing Page | `landing-page/` | Client-side decryptor + branded phishing page |
| Capture Backend | `capture-backend/` | Receives creds, encrypts, logs, redirects |
| CDN Config | `cdn-config/` | Cloudflare Worker reverse proxy |
| URL Generator | `scripts/` | Assembles phishing URLs |

## Detection Evasion

- URL fragment (`#`) is never sent to the server — invisible to network scanners
- AES-256-GCM with random nonce per victim — no static signatures possible
- CDN fronting — victims see Cloudflare TLS cert, not your origin
- Each URL is unique — different nonce = different ciphertext per victim

## Requirements

- Go 1.22+ (payload-generator, capture-backend)
- Node.js 18+ (cdn-config — Cloudflare Wrangler)
- Python 3 (test server for landing page)
- Cloudflare account (optional — for CDN fronting)

## Warning

This is a security research and education tool. Use only for authorized
phishing simulations, penetration testing, and red team operations with
explicit written authorization.
