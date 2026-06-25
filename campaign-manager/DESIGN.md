# Campaign Manager — Design System

Last updated: 2025-06-25 (reconciled with app.html post-Phase 2 SPA redesign)

## Product Context

- **What this is:** A self-hosted phishing simulation campaign manager for red team operators.
- **Who it's for:** Security professionals running credential harvesting assessments.
- **Project type:** Web app (SPA) + TUI (terminal). Dark theme only.
- **Mood:** Utilitarian, precise, intentional. "This is serious software for serious work."

## Aesthetic Direction

- **Direction:** Industrial/Utilitarian — function-first, data-dense, monospace accents
- **Decoration level:** Minimal — typography and spacing do the work, no ornament
- **Typography-first:** IBM Plex Sans carries the hierarchy, JetBrains Mono marks data

## Color Tokens

| Token | Value | Contrast on --bg | Usage |
|-------|-------|------------------|-------|
| --bg | #090c10 | — | Page background |
| --surface | #11161e | — | Cards, panels, elevated surfaces |
| --border | #1e2530 | — | Borders, dividers |
| --text | #b0b8c0 | 5.3:1 (AA) | Body text |
| --text-bright | #e8ecf1 | 13.8:1 (AAA) | Headings, emphasized text |
| --blue | #4199f5 | — | Primary actions, links, focus rings |
| --green | #26a641 | — | Success states, completed steps |
| --purple | #8b6ff0 | — | Info, verified state |
| --amber | #e8953b | — | Warnings, in-progress states |
| --red | #f5534b | — | Errors, destructive actions |

**Gap:** `--danger-bg` and `--success-bg` tokens are not yet defined. Toast CSS hardcodes `#1a6b30` (success) and `#9b1c1c` (error) backgrounds. Define `--danger-bg: rgba(245,83,75,0.15)` and `--success-bg: rgba(38,166,65,0.15)` and refactor toast styles to use them.

## Typography

| Role | Font | Weight | Size | Line-height |
|------|------|--------|------|-------------|
| Display/headings | IBM Plex Sans | 600 | 18-24px | — |
| Body | IBM Plex Sans | 400 | 14px | 1.5 |
| Labels/captions | IBM Plex Sans | 500 | 12-13px | — |
| Code, paths, IDs | JetBrains Mono | 400-500 | 11-13px | — |
| Data values | JetBrains Mono | 500 | 13px | — |

CSS variables:
```css
--font-sans: 'IBM Plex Sans', -apple-system, sans-serif;
--font-mono: 'JetBrains Mono', 'SF Mono', monospace;
```

Loaded from Google Fonts via `@import` with `display=swap` to avoid FOUT.

## Spacing Scale (4px grid)

| Step | px | Usage examples |
|------|----|---------------|
| 1 | 4 | Gap, tight padding |
| 2 | 8 | Component gap, heading margin-bottom |
| 3 | 12 | Step indicator padding |
| 4 | 16 | Card gap, container padding |
| 5 | 20 | Card padding |
| 6 | 24 | Column gap, container padding |
| 8 | 32 | Container padding |
| 12 | 48 | Top padding on narrow containers |

**Gap:** Some values in the template deviate from the 4px grid: 6px (border-radius, margin), 10px (padding), 14px (font-size, table padding), 18px (font-size), 22px (padding). These should be audited and snapped where practical.

## Layout

- **Approach:** Hybrid — strict two-panel grid for the app workspace
- **Sidebar:** 300px fixed width, campaign list with scroll
- **Main panel:** Flexible, fills remaining width
- **Max content width:** N/A (full-viewport app, no centered container)
- **Breakpoints:** None (desktop-only tool, small operator team)

## Components

### Button

| Variant | Background | Text | Border | Radius | Padding | Font | Min size |
|---------|-----------|------|--------|--------|---------|------|----------|
| Primary | --blue | #fff | none | 6px | 10px 22px | 14px/500 | 44x44px |
| Success | --green | #fff | none | 6px | 10px 22px | 14px/500 | 44x44px |
| Ghost | transparent | --text | 1px --border | 6px | 10px 22px | 14px/500 | 44x44px |
| Small | --blue | #fff | none | 4px | 4px 12px | 12px/500 | 44x44px |

Hover: `opacity: 0.85`. Focus: `outline: 2px solid var(--blue); outline-offset: 2px`.

### Badge

| Status | Background | Text Color |
|--------|-----------|------------|
| Draft | rgba(161,170,179,0.15) | --text |
| Active | rgba(65,153,245,0.15) | --blue |
| Verified | rgba(139,111,240,0.15) | --purple |
| Deployed | rgba(38,166,65,0.15) | --green |

12px font, 600 weight, 12px border-radius, `role="status"`. Padding: 2px 10px.

**Gap:** detail.html badges use 3px 12px padding. Standardize on 2px 10px.

### Card

- --surface background, 1px --border border, 8px radius, 20px padding
- Summary cards: `repeat(auto-fit, minmax(200px, 1fr))` grid, 16px gap
- Card label: 12px, uppercase, 0.5px letter-spacing, --text, 6px margin-bottom
- Card value: 28px, 700 weight, --text-bright

### Panel

- --surface background, 1px --border border, 8px radius, 24px padding
- Heading: 18px (or 22px for step headers), 600 weight, --text-bright
- Description: 13-14px, --text, 18-24px margin-bottom

### StepIndicator

Horizontal flex bar with equal-width steps:
- Default: --surface background, 1px --border border, 12px 8px padding, --text color
- Done: --green border + text, checkmark prefix
- Current: --blue border + text, 600 weight
- First/last children get radius; adjacent borders collapse via `border-left: none`

### Form Input

- --bg background, 1px --border border, 6px radius
- 10px 12px padding, 14px font, --text-bright color, inherit font family
- Focus: `border-color: var(--blue)`
- Label: 13px, 500 weight, --text-bright
- File input: `::file-selector-button` styled with --border background
- Hint text: 12px, --text, 6px margin-top

### Lure Card Grid

Brand selection cards in a flexible grid:
- 2x2 or 3x3 grid layout, 12px gap
- Each card: --surface background, 8px radius, 12px padding, cursor pointer
- Icon square: category-colored background with brand initial
- Selected state: 2px --blue border
- Hover: border-color brightens
- Category badge below brand name

### Toast

Fixed position, top-right, single-column stacking:
- Container: `position: fixed; top: 16px; right: 16px; z-index: 9999`
- Success: `#1a6b30` background, 4s auto-dismiss
- Error: `#9b1c1c` background, 6s auto-dismiss
- Slide-in animation from right edge
- Multiple toasts stack vertically

**Gap:** Background colors should use --success-bg / --danger-bg tokens instead of hardcoded hex values.

### Spinner

16px border-based spinning circle (`@keyframes spin`), used inline on buttons during loading. Button text changes to "Processing..." and button is disabled during spin.

### Progress Bar

Indeterminate animated bar using `::after` pseudo-element with horizontal translation. Shown during CSV file upload. Track: --border color. Bar: --blue.

### Field Error / Warning

Inline text below form fields:
- Error: 12px, --red color, 4px margin-top
- Warning: 12px, --amber color, 4px margin-top
- Errors clear when user types in the associated field

### Deploy Summary Card

Panel variant shown before final deploy:
- Header: "Ready to Deploy" with bottom border
- Rows: two-column grid (label 90px + value flex), label uppercase/muted, value bright
- Link value: monospace, ellipsis truncation
- Buttons: "Download ZIP & Deploy" (green) + "Download ZIP Again" (blue for already deployed)

## States

### Empty

Sidebar: "No campaigns yet. Create your first simulation." with subtle icon + "New Campaign" CTA.

### Loading

- Button: spinner icon + "Processing..." text, disabled during API calls
- Upload: indeterminate progress bar during CSV upload

### Error

- Inline: red text below form fields (empty required fields, invalid key length)
- Toast: API errors appear top-right, auto-dismiss after 6s

### Success

- Toast: confirmation messages appear top-right, auto-dismiss after 4s
- Copy feedback: button text changes to "Copied!" for 2s

## Accessibility

| Feature | Status | Implementation |
|---------|--------|---------------|
| ARIA landmarks | Implemented | `<nav aria-label="Campaign list">`, `<main role="main" id="main-content">`, `<header role="banner">` |
| Skip link | Implemented | "Skip to main content" link, visually hidden until focused |
| Focus rings | Implemented | `:focus-visible { outline: 2px solid var(--blue); outline-offset: 2px }` |
| Touch targets | Implemented | 44px minimum on buttons, cards, campaign items |
| Reduced motion | Implemented | `@media (prefers-reduced-motion: reduce)` disables all animations |
| Color contrast | Implemented | --text: #b0b8c0 on --bg yields 5.3:1 (AA). --text-bright: #e8ecf1 yields 13.8:1 (AAA) |
| Form labels | Implemented | All inputs have visible `<label>` elements with matching `for` attributes |
| Status badges | Implemented | `role="status"` on badges for screen reader announcements |

## Anti-patterns

- No system font stacks as primary display fonts (IBM Plex Sans + JetBrains Mono)
- No purple/violet gradients or blue-to-purple schemes
- No 3-column feature grids with icons in circles
- No centered everything
- No emoji as design elements
- No decorative blobs, floating circles, or wavy dividers
- No colored left-border on cards

## Decisions Log

| Date | Decision | Rationale |
|------|----------|-----------|
| 2025-06-25 | Initial DESIGN.md | Created by /plan-design-review based on existing templates |
| 2025-06-25 | SPA two-panel redesign | Replaced 3 page-based templates with single app.html |
| 2025-06-25 | IBM Plex Sans + JetBrains Mono | Replaced system font stacks — intentional typography for security tool |
| 2025-06-25 | --text bump to #b0b8c0 | AA contrast compliance |
| 2025-06-25 | Full a11y pass | ARIA landmarks, skip link, focus rings, touch targets, reduced motion |
| 2025-06-25 | Interaction states | Toasts, spinners, progress bars, inline validation, copy feedback |
| 2025-06-25 | Deploy confirmation gate | Summary card before final deploy action |
| 2025-06-25 | DESIGN.md reconciled | Updated to reflect post-Phase-2 implementation state |
