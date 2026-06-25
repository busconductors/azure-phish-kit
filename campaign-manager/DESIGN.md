# Campaign Manager — Design System

## Color Tokens

| Token | Value | Usage |
|-------|-------|-------|
| --bg | #090c10 | Page background |
| --surface | #11161e | Cards, panels, elevated surfaces |
| --border | #1e2530 | Borders, dividers |
| --text | #a1aab3 | Body text |
| --text-bright | #e8ecf1 | Headings, emphasized text |
| --blue | #4199f5 | Primary actions, links, focus rings |
| --green | #26a641 | Success states, completed steps |
| --purple | #8b6ff0 | Info, verified state |
| --amber | #e8953b | Warnings, in-progress states |
| --red | #f5534b | Errors, destructive actions |

**Gap:** Spec calls for bumping `--text` to `#b0b8c0` for AA contrast (4.5:1 on `--bg`). Templates currently use `#a1aab3`. The darker value appears in all three templates and should be updated project-wide when contrast is addressed.

**Gap:** Spec defines `--danger-bg: rgba(245,83,75,0.15)` and `--success-bg: rgba(38,166,65,0.15)` tokens. These are not yet present in any template. They will be needed when toast and inline error/success states are implemented.

## Typography

| Role | Font | Weight | Size |
|------|------|--------|------|
| Display/headings | System sans-serif | 600 | 24px |
| Body | System sans-serif | 400 | 14px |
| Labels/captions | System sans-serif | 500 | 12px |
| Code, paths, IDs | SF Mono / Fira Code | 400 | 13px |
| Data values | System sans-serif | 500 | 13px |

Font stack: `-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif`
Monospace stack: `"SF Mono", "Fira Code", monospace`

**Gap:** Spec calls for IBM Plex Sans as the UI font and JetBrains Mono as the code font. Templates use system font stacks instead. The system stack provides instant rendering (no FOUT) and native platform feel; IBM Plex would require a web font import with the associated latency and caching strategy. If IBM Plex Sans is adopted, add `@import url('https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600&family=JetBrains+Mono:wght@400;500&display=swap');` and update all font-family declarations.

## Spacing Scale (4px grid)

| Step | px |
|------|----|
| 1 | 4 |
| 2 | 8 |
| 3 | 12 |
| 4 | 16 |
| 5 | 20 |
| 6 | 24 |
| 8 | 32 |
| 12 | 48 |

**Gap:** Templates use some values outside the 4px grid — `6px` (label margin-bottom, button radius, form-group gap), `10px` (button horizontal padding, input padding), `14px` (table cell padding), `18px` (h2 font-size, toolbar heading), `22px` (button horizontal padding), `28px` (h1 margin-bottom, form-card padding). These should be audited and snapped to grid values where practical, but the core rhythm (4, 8, 12, 16, 20, 24, 32, 48) is the system baseline.

## Components

### Button

Three variants are implemented across the templates:

- **Primary:** `--blue` background, `#fff` text, 6px radius, 10px 22px padding, 14px font, 500 weight
- **Success:** `--green` background, `#fff` text (same dimensions). Used only in detail.html for the Deploy action.
- **Ghost:** transparent background, `--text` color, 1px `--border` border (same dimensions). Used only in new.html for Cancel.
- **Small:** `4px 12px` padding, `12px` font, `4px` radius. Used for table action buttons in list.html.
- **Hover:** `opacity: 0.85`
- **Transition:** `opacity 0.15s`

**Gap:** Spec calls for a minimum touch target of 44px by 44px and a visible focus ring (`2px --blue outline, 2px offset` on `:focus-visible`). Neither is implemented in the current templates. Spec also calls for a `btn-success` variant with identical dimensions to `btn-primary` — this matches, though only the green background color class exists; no other success-specific properties are defined.

### Badge

Four status variants, all using 12px font, 600 weight, 12px radius:

| Class | Background | Text Color |
|-------|-----------|------------|
| badge-draft | `rgba(161,170,179,0.15)` | `--text` |
| badge-active | `rgba(65,153,245,0.15)` | `--blue` |
| badge-verified | `rgba(139,111,240,0.15)` | `--purple` |
| badge-deployed | `rgba(38,166,65,0.15)` | `--green` |

**Gap:** Padding is inconsistent — list.html uses `2px 10px`, detail.html uses `3px 12px`. Standardize on one (spec calls for `2px 10px`).

### Card

- `--surface` background, 1px `--border` border, 8px radius, 20px padding
- Summary cards use `grid-template-columns: repeat(4, 1fr)` with 16px gap
- Card label: 12px, uppercase, 0.5px letter-spacing, `--text`, 6px margin-bottom
- Card value: 28px, 700 weight, `--text-bright`
- Color-modifier classes: `.green` (--green), `.blue` (--blue), `.purple` (--purple), `.amber` (--amber)

**Gap:** Spec calls for `repeat(auto-fit, minmax(200px, 1fr))` on summary cards. Templates use a fixed `repeat(4, 1fr)` which does not reflow on narrow viewports. The `auto-fit` pattern should be adopted for responsiveness.

### Panel

- `--surface` background, 1px `--border` border, 8px radius, 24px padding
- Heading (h2): 18px, 600 weight, `--text-bright`, 8px margin-bottom
- Description (`.desc`): 14px, `--text`, 20px margin-bottom

### StepIndicator

A horizontal 5-step progress bar implemented in detail.html:

- Container: `display: flex`
- Each step: `flex: 1`, center-aligned text, 12px 8px padding, `--surface` bg, 1px `--border` border, 12px font, 500 weight, `--text` color
- Done state: `--green` color and border, checkmark prefix (`&check;`)
- Current state: `--blue` color and border, 600 weight
- First child: `border-radius: 6px 0 0 6px` (left radius)
- Last child: `border-radius: 0 6px 6px 0` (right radius)
- Adjacent borders collapse: `border-left: none` on all but `:first-child`

### Form Input

- `--bg` background, 1px `--border` border, 6px radius
- 10px 12px padding, 14px font, `--text-bright` color, `font-family: inherit`
- Focus: `border-color: --blue`
- Transition: `border-color 0.15s`
- Label: 13px, 500 weight, `--text-bright`, 6px margin-bottom
- Select: same styling as input; options get `--surface` background, `--text-bright` color
- File input: 8px padding; `::file-selector-button` gets `--border` background, `--text-bright` color, 4px 12px padding, 4px radius

### Toast (not implemented)

**Gap:** Spec defines toast position (top-right, fixed), success variant (`--success-bg` bg, `--green` text), and error variant (`--danger-bg` bg, `--red` text). No toast component exists in the current templates.

## States

### Empty

- Centered (`text-align: center`), padding `48px 0`, `--text` color
- Contextual message ("No campaigns yet.") + primary CTA button
- **Gap:** Spec requires that every empty state names what's missing and what to do — the current implementation meets this for the campaign list but would need the same pattern for any future empty states.

### Loading (not implemented)

**Gap:** Spec defines three loading patterns — button spinner with "Processing..." text (disabled state), page skeleton cards (pulsing `--surface` animation), and upload progress bar (`--blue` fill, `--border` track). None are yet implemented.

### Error (not implemented)

**Gap:** Spec defines three error patterns — inline below form fields (`--red` text, 12px font), toast for API errors (top-right, auto-dismiss 5s), and form-level errors in a banner above the submit button. None are yet implemented.

## Accessibility

**Current state:** The templates have minimal accessibility support — no ARIA landmarks, no focus-visible styles, no skip link, no `prefers-reduced-motion` media query, and touch targets below 44px on small buttons.

**Gap from spec:**

| Feature | Spec | Template |
|---------|------|----------|
| ARIA landmarks | `<main>`, `<nav>`, `<header role="banner">` | Not present |
| Visible focus | `2px --blue outline, 2px offset` on `:focus-visible` | Not present |
| Touch targets | 44px minimum | Not enforced |
| Reduced motion | `prefers-reduced-motion: disable all transitions/animations` | Not present |
| Color contrast | AA (4.5:1) on all text | `--text` value `#a1aab3` likely below AA on `--bg` |
| Skip link | "Skip to main content" at top of page | Not present |

## Dark Theme Only

This is a security operations tool. Light theme is not needed. All colors are designed for dark backgrounds. No light-theme variables or media queries exist in the codebase.

## Layout

- **Max-width:** 1100px (list), 960px (detail), 600px (new)
- **Container padding:** 32px 24px (list, detail), 48px 24px (new)
- **Two-column layout (detail):** `grid-template-columns: 1fr 300px`, 24px gap
- **Table:** full-width, collapsed borders, 14px cells, `--border` row dividers, row hover highlights with `--surface` background
- **Table headers:** 12px, 500 weight, uppercase, 0.5px letter-spacing, `--text` color, 10px 14px padding

## Anti-patterns

- No system font stacks as primary display fonts (currently violated — templates use system stacks; spec calls for IBM Plex Sans)
- No purple/violet gradients or blue-to-purple schemes
- No 3-column feature grids with icons in circles
- No centered everything (text-align: center on all elements)
- No emoji as design elements
- No decorative blobs, floating circles, or wavy dividers
- No colored left-border on cards (border-left: 3px solid accent)
