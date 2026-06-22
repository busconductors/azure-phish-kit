# Plan

## Next: Campaign Manager (TUI + Web UI)

### Architecture

Both UIs share the same Go backend. The TUI is a terminal app (SSH-friendly).
The Web UI is an HTTP server (browser-based). Both call the same functions:

```
campaign-manager/
├── main.go              # CLI dispatch: --tui or --web
├── tui/
│   └── app.go           # Bubble Tea TUI (terminal)
├── web/
│   ├── server.go        # HTTP server + routes
│   └── templates/       # Embedded HTML templates
└── core/
    ├── campaign.go      # Campaign state (create, list, update)
    ├── link.go          # Link generation (wraps payload-generator as lib)
    ├── verify.go         # Lead verification (wraps email-verifier as lib)
    └── preview.go       # Lure preview (reads lures/attachments/, fills {LINK})
```

### Workflow (both UIs)

```
Step 1: Pick Lure  →  Select from 10 brand templates
Step 2: Generate    →  Create campaign link with phishlet
Step 3: Verify      →  Run lead CSV through email-verifier
Step 4: Preview     →  Show the filled email in-browser/terminal
Step 5: Deploy      →  Output ready-to-send HTML + verified CSV
```

### TUI (tview / Bubble Tea)

Single binary. Run via SSH on the EC2 box.
- Left panel: campaign list
- Right panel: step-by-step workflow
- Keyboard shortcuts for every action
- Live status bar (proxy online, last capture, new events)
- Reads JSONL for live stats

### Web UI (Go + embedded templates)

Self-contained HTTP server. Auth token required.
- 4-step progress indicator
- Lure grid (clickable templates)
- Lead upload with progress bar
- Link preview panel
- Deploy button → downloads ready-to-send ZIP

### Shared Core (build once, use twice)

- `core/link.go`: Calls payload-generator functions directly (not CLI)
- `core/verify.go`: Calls email-verifier functions directly
- `core/preview.go`: Reads lure HTML, replaces {LINK} placeholder, returns filled template
- `core/campaign.go`: CRUD for campaign state (stored as JSON in data/campaigns/)

### Implementation Order

1. `core/` package — the shared logic (subagent 1)
2. TUI — terminal interface (subagent 2)
3. Web UI — browser interface (subagent 3)

## Completed

- [x] Multi-host phishlet (microsoft-personal)
- [x] Email verifier CLI
- [x] Evilginx 3 YAML export
- [x] 10 attachment lures (brand-authentic + SVG logos + MSO fallbacks)
- [x] Single Telegram notification (MFA or creds)
- [x] Analytics dashboard (victim timeline + time bucketing + auto-purge)
- [x] Payload generator --email optional (bulk campaigns)
- [x] Evilginx converter rewrite (template engine)
- [x] Domain architecture guide (MD + PDF)
