# Aged Domain Acquisition Guide — STRASSER ⛫ LAB

**Classification:** Internal | **Date:** 2026-06-21

---

## Why Aged Domains Matter

Email gateways check domain age. A domain registered yesterday gets a high spam score before the email is even opened. A domain registered 3+ years ago passes the age check silently.

| Domain Age | Spam Score Impact | Inbox Rate |
|-----------|------------------|------------|
| 0–30 days | Heavy penalty | 30–50% |
| 1–6 months | Moderate penalty | 50–70% |
| 6–12 months | Light penalty | 70–85% |
| 3+ years | No penalty | 85–95% |
| 5+ years | Trust bonus | 90–98% |

---

## Best Sources

### 1. ExpiredDomains.net — Free Search (Start Here)

**URL:** https://expireddomains.net

This is the industry-standard free tool. 500,000+ expired and expiring domains, updated daily.

**Filter setup:**
```
TLD:          .com, .net, .org, .cc
Age:          3+ years
Backlinks:    10 minimum
Price:        $0–$200
Exclude:      Adult, Gambling, Pharmacy, Casino
Sort by:      Domain Age (oldest first)
```

**How to evaluate a candidate:**
1. Copy the domain name
2. Open https://web.archive.org — paste the domain
3. Check what the site USED to be. If it was a real business, blog, or local service — good. If it was a spam page or parked domain with ads — skip.
4. Run through https://mxtoolbox.com/blacklists.aspx — must show 0/100 blacklists
5. Check USPTO for trademarks: https://www.uspto.gov/trademarks/search

### 2. GoDaddy Auctions — Largest Inventory

**URL:** https://auctions.godaddy.com

65% of all expired domain sales happen here.

**Filter for bargains:**
```
Type:        Closeout (fixed price, no bidding)
Price:       Under $50
Age:         2+ years
TLD:         .com, .net, .cc
```

Closeout domains are $12 + registration fee. They're domains nobody bid on during the auction phase. Most are garbage but 5–10% are hidden gems — small businesses that shut down, personal blogs that were abandoned, local services that went out of business. These have clean history at a bargain price.

### 3. NotRenewing.com — Flat $99

**URL:** https://notrenewing.com

Launched March 2026. Every domain is $99 fixed price. Minimum 24 months old, expiring within 12 months. Sellers list domains they plan to let expire anyway. Good for finding domains with 3–5 years of clean history at a predictable price.

### 4. NameJet / SnapNames — Backordering

**URL:** https://namejet.com | https://snapnames.com

Place a backorder on an expiring domain. If you're the only bidder, you get it at the reserve price ($69). If multiple bidders, it goes to a 3-day private auction. Best for targeting domains you've already vetted on ExpiredDomains.net.

---

## Step-by-Step Acquisition

### Step 1: Hunt (5 minutes)
1. Open ExpiredDomains.net
2. Apply filters: .com/.net/.cc, age 3+, backlinks 10+, price under $200
3. Open 20 candidates in tabs

### Step 2: Vet (2 minutes per domain)
1. Web Archive check — was it a real site?
2. Blacklist check — is it clean?
3. Trademark check — is it safe to use?
4. Backlink check — are links from real sites or spam farms?

### Step 3: Buy
1. GoDaddy Closeout: click "Buy Now" → checkout → domain in your account in minutes
2. NotRenewing: click "Buy" → pay $99 → domain transferred within 24 hours
3. NameJet backorder: place bid → wait for domain to drop → if won, domain in account in 24 hours

### Step 4: Transfer to Cloudflare
1. After purchase, unlock the domain at your registrar
2. Get the transfer authorization code
3. Cloudflare Dashboard → Add Site → Transfer Domain
4. Enter the code → pay 1 year renewal → domain lives on Cloudflare
5. Set up DNS + Worker (our standard deployment)

---

## Red Flags — Do NOT Buy

- **Parked with ads** — the domain was never a real site, just a landing page with ads. No backlink value, likely flagged.
- **Pharmacy/casino/adult history** — even if clean now, these niches trigger spam filters permanently.
- **Trademarked name** — using "microsoft-support.cc" gets you sued and your domain seized.
- **Spam score above 5%** — Moz Spam Score or similar. Check before buying.
- **Dropped multiple times** — WHOIS shows repeated creation dates. The domain was repeatedly abandoned.
- **Chinese/Japanese characters in backlinks** — common sign of a spam network.

---

## Quick Reference

```
ExpiredDomains.net     — Free domain hunting (start here)
GoDaddy Auctions       — Closeout bargains ($12+)
NotRenewing.com        — Flat $99 aged domains
NameJet                — Backordering (pay only if you win)
web.archive.org        — Check what a domain used to be
mxtoolbox.com          — Blacklist check
uspto.gov              — Trademark search
```

---

## Budget: $50–$150 gets you a solid aged domain

| Stage | Cost |
|-------|------|
| GoDaddy Closeout purchase | $12–$50 |
| Transfer to Cloudflare | $8–$10 (1 year renewal) |
| **Total** | **$20–$60** |

For $150 you can get a 5+ year aged .com with clean history and 20+ backlinks. Worth every dollar for the inbox rate improvement.
