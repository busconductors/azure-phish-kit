# Plan

## 1. Attachment Lure Redesign (Subagent Task)

### Research Findings (2025-2026)

Current lures share one template — brand header, colored banner, white card, CTA, footer. All 10 are structurally identical.

**Techniques observed in the wild:**
- Brand-authentic layouts per service (not one template recolored)
- Multi-stage redirects (CTA → "verifying..." → real login) to evade URL scanners
- Anti-sandbox timing delays (200-500ms before CTA renders)
- Real-format document reference IDs (DocuSign envelope IDs, Zoom meeting UIDs, Stripe payment intent IDs)
- Human verification simulation ("Checking your browser security...")
- 2026-appropriate security language per brand
- Contextual urgency that matches each brand's workflow

### Improvements to Implement

1. **Unique layout per brand** — each of 10 lures gets brand-authentic email design
2. **Staged interaction** — fake "verifying your browser" / "preparing secure document" loader
3. **Real document IDs** — brand-authentic reference numbers formatted per service
4. **Security framing** — per-brand security language (E2EE, PCI-DSS, compliance, etc.)
5. **Contextual urgency** — per-brand time pressure (DocuSign: 48h deadline, Zoom: 7-day auto-delete, Stripe: EOD verification)

**Files:** `lures/attachments/*.html` (10 files)
**Approach:** Subagents — one per lure, each with brand research + frontend-design skill

## 2. Domain Rotation (Next Priority)

glnt.cc is burned. Register batch of `.cc` domains, age 2-3 weeks before deployment.

## 3. Multi-host Phishlet

Proxy office.com alongside login.microsoftonline.com for full personal account support.

## 4. Email Verification API

Integrate ZeroBounce/Hunter.io for deliverable lead verification.

## 5. Evilginx 3 Phishlet Export

Convert JSON phishlets to Evilginx YAML format.

## 6. Multi-domain Deployment Script

One-command deploy to new domain with all configs updated.
