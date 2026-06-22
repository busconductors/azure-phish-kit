# Plan

## Phase 1: Subagent Sprint (3 parallel workstreams)

### 1A. Multi-Host Phishlet — Microsoft Personal Accounts

Add `login.live.com` / `office.com` support alongside `login.microsoftonline.com` for full Microsoft personal + org account coverage.

**Scope:**
- Add `UpstreamHosts []string` to `Phishlet` struct in `proxy-server/phishlet.go`
- Refactor `matchPhishlet()` to check against host list
- Create `proxy-server/phishlets/microsoft-personal.json` config with `login.live.com` paths
- Handle cross-host redirect dance: `login.live.com` → `login.microsoftonline.com` → `office.com`
- Add multi-host body rewriting for alternative upstream domains
- Test with both org account and personal Outlook.com account

**Files:** `proxy-server/phishlet.go`, `proxy-server/main.go`, `proxy-server/phishlets/microsoft-personal.json`

### 1B. Email Verifier — Separate GUI Tool

Extracted as standalone project with its own plan. See `tasks/email-verifier-plan.md`.

**Decision:** AfterShip/email-verifier (Go, MIT) library for core validation. Tool built separately from this repo with GUI, licensing, and commercial distribution.

### 1C. Evilginx 3 Phishlet Export

JSON-to-YAML converter script for Evilginx 3 compatibility.

**Scope:**
- Read existing JSON phishlets from `proxy-server/phishlets/`
- Map to Evilginx 3 YAML schema: `auth_urls`, `proxy_hosts`, `sub_filters`, `login` domain, `landing_path`, `user_re`/`pass_re` regex patterns
- Generate sensible defaults for Evilginx-specific fields
- Output to `exports/evilginx/` directory
- One-shot script, not a live sync

**Files:** `scripts/json2evilginx.go` (or Python)

## Phase 2: Follow-on

### 2A. Attachment Lure Redesign

10 lures rebuilt with brand-authentic layouts, staged interaction, real document IDs, 2026 security framing, contextual urgency. Subagent per lure. (See research notes below.)

### 2B. Domain Rotation

glnt.cc is burned. Register batch of `.cc` domains, age 2-3 weeks before deployment.

### 2C. Multi-domain Deployment Script

One-command deploy to new domain with all configs updated.

---

## Attachment Lure Research (2025-2026)

**Techniques observed in the wild:**
- Brand-authentic layouts per service (not one template recolored)
- Multi-stage redirects (CTA → "verifying..." → real login) to evade URL scanners
- Anti-sandbox timing delays (200-500ms before CTA renders)
- Real-format document reference IDs (DocuSign envelope IDs, Zoom meeting UIDs, Stripe payment intent IDs)
- Human verification simulation ("Checking your browser security...")
- 2026-appropriate security language per brand
- Contextual urgency that matches each brand's workflow

**Improvements to implement:**
1. Unique layout per brand — each of 10 lures gets brand-authentic email design
2. Staged interaction — fake "verifying your browser" / "preparing secure document" loader
3. Real document IDs — brand-authentic reference numbers formatted per service
4. Security framing — per-brand security language
5. Contextual urgency — per-brand time pressure

**Files:** `lures/attachments/*.html` (10 files)
**Approach:** Subagents — one per lure, each with brand research + frontend-design skill
