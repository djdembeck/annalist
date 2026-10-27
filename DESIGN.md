---
name: Annalist
description: Self-hosted AI release-note automation for GitHub and Forgejo/Gitea — the Quiet Workspace.
colors:
  canvas: "#f5eee5"
  surface: "#fff"
  control-hover: "#fffaf4"
  line: "#e6dccf"
  line-strong: "#9c8768"
  ink: "#302b26"
  muted: "#6f665c"
  copper: "#bd431f"
  copper-deep: "#a03616"
  focus: "#bd431f"
  primary-ink: "#fff"
  selection: "#eadbc9"
  skeleton: "#e0d2bf"
  scroll-thumb: "#b9a995"
  published: "#263524"
  published-bg: "#edf3ea"
  published-border: "#b7c9b1"
  published-icon: "#2c5a33"
  failed: "#482a22"
  failed-bg: "#f8eae5"
  failed-border: "#d9b1a2"
  failed-icon: "#8c351e"
  cached: "#3c342b"
  cached-bg: "#eee8df"
  cached-border: "#c9bda9"
  cached-icon: "#62533d"
typography:
  display:
    fontFamily: "\"Avenir Next\", \"Segoe UI\", Cantarell, system-ui, sans-serif"
    fontSize: "31px"
    fontWeight: 650
    lineHeight: 1.16
    letterSpacing: "-0.028em"
  heading-sm:
    fontFamily: "\"Avenir Next\", \"Segoe UI\", Cantarell, system-ui, sans-serif"
    fontSize: "21px"
    fontWeight: 650
    lineHeight: 1.3
    letterSpacing: "-0.015em"
  body:
    fontFamily: "\"Avenir Next\", \"Segoe UI\", Cantarell, system-ui, sans-serif"
    fontSize: "15px"
    fontWeight: 400
    lineHeight: 1.5
  text:
    fontFamily: "\"Avenir Next\", \"Segoe UI\", Cantarell, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.3
  label:
    fontFamily: "\"Avenir Next\", \"Segoe UI\", Cantarell, system-ui, sans-serif"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: 1.4
  mono:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.65
rounded:
  xs: "4px"
  sm: "5px"
  md: "7px"
  control: "8px"
  surface: "12px"
spacing:
  "0": "3px"
  "0.5": "5px"
  "1": "7px"
  "1.5": "8px"
  "2": "10px"
  "2.5": "12px"
  "3": "15px"
  "4": "16px"
  "5": "20px"
  "6": "24px"
  "7": "28px"
  "8": "32px"
components:
  button-primary:
    backgroundColor: "{colors.copper}"
    textColor: "{colors.primary-ink}"
    typography: "{typography.text}"
    rounded: "{rounded.control}"
    padding: "9px 15px"
    height: "44px"
  button-primary-hover:
    backgroundColor: "{colors.copper-deep}"
  button-secondary:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    typography: "{typography.text}"
    rounded: "{rounded.control}"
    padding: "9px 15px"
    height: "44px"
  button-secondary-hover:
    backgroundColor: "{colors.control-hover}"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.muted}"
    typography: "{typography.text}"
    rounded: "{rounded.control}"
    padding: "9px 15px"
    height: "44px"
  field:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    typography: "{typography.text}"
    rounded: "{rounded.control}"
    padding: "9px 12px"
    height: "44px"
  badge:
    backgroundColor: "{colors.cached-bg}"
    textColor: "{colors.cached}"
    typography: "{typography.text}"
    rounded: "{rounded.sm}"
    padding: "3px 8px"
  panel:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    rounded: "{rounded.surface}"
    padding: "24px"
---

# Design System: Annalist

<!-- Quiet Workspace world, approved after the direction round. The built dashboard is the authority for every value in this file; light values are the `:root` defaults, dark values the `[data-theme="dark"]` overrides recorded in `.impeccable/design.json`. -->

## Overview

**Creative North Star: "The Quiet Workspace"**

Annalist is a warm working interface for release outcomes and automation configuration. A warm paper canvas holds a white work plane in light mode; tobacco charcoal and cream in dark mode. The operator's job is comparison — a repository, its latest observed generation, its configured triggers, and a way to act — so the layout behaves like a register: aligned columns at desktop width, labeled rows below 980px, and nothing on the screen competing with the data.

Copper (`--rust`) marks the actions that commit work; the ledger mark and wordmark give the product its identity. Result badges are the system's voice: high-contrast text over a restrained tint, a same-hue border, and a small 14px icon, so that Published, Cached, Failed, and the other run states read at a glance without noise. Configuration is never presented as a health claim, and the empty states say what is empty rather than decorate it.

**Key Characteristics:**
- Warm paper canvas (#f5eee5) with white work planes in light; tobacco charcoal (#211d19) with cream (#f4eade) ink in dark.
- One humanist system UI stack (Avenir Next / Segoe UI / Cantarell / system-ui) for everything; no display font, no serif.
- Copper for committed actions on 12px working surfaces; quiet 44px-tall top navigation.
- Result badges: tint + border + 14px stroke icon + bold text, one hue family per outcome.
- 44px minimum gesture height, tabular numerals, wrapping text — information is never clipped.

## Colors

Two complete plans share one token set: the light paper plan is the `:root` default; the dark charcoal plan is `:root[data-theme="dark"]`, toggled by the top-right switch, stored in `localStorage`, and defaulted from `prefers-color-scheme` by an inline script before first paint. Every value below is the light plan; the matching dark value is carried in `.impeccable/design.json` beside each token.

### Primary
- **Copper** (`#bd431f` / dark `#e8a17b`): the committed action — primary buttons, the ledger mark's stamped bookmark, caret, checkbox accent, running-state badge text, markdown links. Its hover is **Copper Deep** (`#a03616` / dark `#d88b63`). In dark mode the copper lightens so it sits on charcoal; the white primary ink flips to near-charcoal (`#241d18`).
- **Focus** (`#bd431f` / dark `#f1b28b`): the 3px keyboard-focus outline and its 3px offset. Dark mode uses the lighter tone; light mode reuses the copper itself.

### Neutral
- **Canvas** (`#f5eee5` / dark `#211d19`): the warm paper page behind the shell.
- **Surface** (`#fff` / dark `#2c2621`): the white work plane — panels, fields, buttons, tables, the top bar.
- **Control Hover** (`#fffaf4` / dark `#393029`): one step of warmth for control hover, soft panels, the register's detail rows, selection state, and text-pair highlights.
- **Line** (`#e6dccf` / dark `#53473d`): 1px hairlines — row dividers, borders, blockquote rails.
- **Line Strong** (`#9c8768` / dark `#8d7d6e`): control and field edges; its hover is `#8a7355` / dark `#ad9682`.
- **Ink** (`#302b26` / dark `#f4eade`): primary text; **Muted** (`#6f665c` / dark `#c6b6a5`) carries secondary text, placeholders, table headers, and labels.
- **Selection** (`#eadbc9` / dark `#66513f`): text selection fill, active tab/choice background, light highlight plane.
- **Skeleton** (`#e0d2bf` / dark `#4a4036`): loading placeholder fill. **Scroll Thumb** (`#b9a995` / dark `#8d7d6e`): scrollbar fill.

### Status
- **Published green** — text `#263524` / dark `#edf4e9`, background `#edf3ea` / dark `#2e3b2d`, border `#b7c9b1` / dark `#64765b`, icon `#2c5a33` / dark `#accc97`. The observed generation outcome: published, complete, accepted runs.
- **Failed clay** — text `#482a22` / dark `#ffebe3`, background `#f8eae5` / dark `#492f29`, border `#d9b1a2` / dark `#a16d5a`, icon `#8c351e` / dark `#f3b49a`. Observed errors, invalid input, interrupted runs.
- **Cached neutral** — text `#3c342b` / dark `#f5eadb`, background `#eee8df` / dark `#3c332a`, border `#c9bda9` / dark `#89745e`, icon `#62533d` / dark `#d6bd99`. Cached, preserved, and quiet states.
- **Running** is copper text on the surface with a copper border — an in-progress cue, not a success color. Preserved takes the cached tint with a **dashed** border so it reads as kept-in-place, not failed; generated and empty take ink on the surface with a strong border. A quiet row state — muted text on the control-hover wash with a strong border — carries the "Not configured" platform fact; the configured fact ("Configured") renders in the published family as a plain server fact.

### Named Rules

**The Warm Assurance Rule.** Every state ships a text, a background, a border, and an icon in one hue family. Color tint is support; the word and icon carry the meaning, so no badge relies on hue alone.

**The Copper Commit Rule.** Copper is the committed action and the identity markup. It is not a second primary per surface, not a link color outside generated markdown, and not a hover-wash or decoration on neutral work.

**The Configuration Is Not a Health Claim Rule.** Configured / not-configured is a server configuration fact, stated as a server fact: the "Configured" label wears the published family and "Not configured" the muted wash. It is not a connectivity claim — the UI does not probe reachability, and no endpoint is reported reachable here. Wording follows the same line: a platform is "configured" on the server; an endpoint is "reachable" only as a troubleshooting hint when a run has actually failed.

**The Dual-Plan Rule.** A token is only valid when both plans exist. A light-mode rule that breaks on charcoal — white text on copper, hard shadows, borders darker than the charcoal below — is a defect.

## Typography

**Work Font:** `Avenir Next`, `Segoe UI`, Cantarell, `system-ui` — the single stack for display, body, and labels.
**Mono Font:** `ui-monospace`, SFMono-Regular, Menlo, Consolas — for code and commands only.

**Character:** One humanist system stack at working sizes; the hierarchy is made by weight and size, not face. Tabular numerals throughout keep the register's columns aligned.

### Hierarchy
- **Page heading (h1).** Weight 650, 31px, line-height 1.16, letter-spacing -0.028em; 27px below 980px. One per view.
- **Panel heading (h2).** Weight 650, 21px, line-height 1.3, letter-spacing -0.015em.
- **Subheading (h3).** Weight 620, 18px, line-height 1.35.
- **Body.** Weight 400, 15px, line-height 1.5 — the base document size.
- **Working text.** 14px; the size of buttons, fields, table cells, badges, and ledes (section ledes capped at 75ch, muted).
- **Label.** 13px, muted: table column labels, chips, run metadata, hints. Row labels in the mobile register are 13px at weight 550.
- **Code.** Mono, 14px inside rendered markdown; inline code carries the control-hover fill at 4px radius.

### Named Rules

**The Working-Text Rule.** Only the two named stacks exist. Weights 400, 550, 600, 620, and 650 do the work; 650 is the upper bound and no weight above it appears.

## Layout

A centered 1200px shell (`max-width: 1200px`, `margin: auto`) with 32px inline gutters — 16px below 980px and 20px for the top bar on mobile. The top bar is a hairline-capped band: 40px ledger mark, 29px wordmark (weight 650, letter-spacing -0.035em), quiet nav links, and the theme switch at the right.

The mainspace rhythm is 20px between workspace blocks; a panel's own interior is 24px (20px below 600px). The repository register uses fixed columns — Repository 27%, Latest generation 29%, Automation 19%, Actions 25% — with 22px cell padding relaxing to 14px between 980–1100px. Row content sits on a 19px/18px vertical pad under a 1px divider; detail rows (failure copy, run history) inset under the Repository column at `calc(27% + 22px)`.

At 980px the register becomes labeled rows: column headers drop, each cell gains its `data-label` in 13px muted text, the row lays out as two columns (name + latest, automation + actions), and section action buttons stretch full-width. At 600px field grids and stacked proofs collapse to one column. 44px is the minimum gesture height everywhere — nav links, tabs, checks, summaries, row actions — and long names wrap at the namespace slash via `overflow-wrap: anywhere` rather than truncate.

## Elevation & Depth

Depth is a hairline plus a whisper of shadow. Panels and grouped containers (`plane`) carry `box-shadow: 0 1px 2px rgba(48,43,38,0.06), 0 10px 24px rgba(48,43,38,0.045)` in light and `rgba(0,0,0,0.28)` / `rgba(0,0,0,0.2)` in dark. Soft panels, banners, detail rows, and badges are flat — their depth comes from the tinted plane and 1px border. Controls, chips, and tables never cast.

### Shadow Vocabulary
- **Panel** (`0 1px 2px var(--shadow), 0 10px 24px var(--shadow-soft)`): the single ambient lift — paper working on the canvas. Nothing else.

### Named Rules

**The Paper-On-Paper Rule.** Only grouped working surfaces lift; the lift is decorative atmosphere, not emphasis. State is never expressed by elevation.

## Shapes

The radius ladder is 4px (inline code), 5px (badges, chips), 7px (theme switch), 8px (controls, fields, mode choices, setup steps), 11px (mobile planes, where inset content unsticks the corner), and 12px (panels, banners, grouped registers). Borders are 1px: line for structure, line-strong for controls. The system has no clipped silhouettes and no pill controls.

## Components

### Buttons
- **Shape:** 8px radius, 44px minimum height, 1px line-strong border, 14px text.
- **Primary:** copper fill, white ink (dark plan flips to near-charcoal on lightened copper), weight 600; hover deepens to copper-deep. One per view — the committed action.
- **Secondary:** surface fill, ink text, strong border; hover warms to control-hover with a deeper border.
- **Ghost:** transparent, muted text, no border; hover shifts to ink. Row **action links** are the 14px underlined variant — transparent at rest, underline appearing on hover.
- **Disabled:** 55% opacity, `not-allowed` cursor, transitions suspended.

### Result Badges
- **Style:** 3px 8px padding, 5px radius, 1px same-hue border, 14px text at weight 650, and a 14px stroke icon (`stroke-width: 1.8`, round caps/joins, `currentColor`) — check, triangle-alert, clock, stacked pages, shield, stop-square, document.
- **States:** the published / failed / cached hue families above; running is copper-bordered on surface; preserved is the cached tint with a dashed border; generated and empty are ink on surface; muted is control-hover on surface.
- **Behavior:** badge text is the state word ("Published", "Cached", "No runs yet"); the icon never carries meaning alone.

### Chips, Tabs, and Choices
- **Chip:** 13px muted text on surface, 5px radius, 1px line border — compact metadata, not a control.
- **Tabs and mode choices:** 44px control height, 8px radius, strong border, surface fill; the active one gets the selection fill with a copper border. Choices that remove data (`--danger`) swap to the failed tint.
- **Setup steps:** 44px tabs where the current step carries the selection fill; completed steps are plain text.

### Register Table / Labeled Rows
- **Desktop:** fixed-column table on a 12px-radius plane; 14px row labels in muted, 15px repository names at weight 620 with the platform in 14px muted below; version weight 550, when-line 14px muted.
- **Mobile (≤980px):** header collapses; each cell renders its `data-label`; the row is a two-column grid with row actions bottom-aligned. Detail rows unstick to full width with 16px padding.
- **Row state:** hover is reserved for text and borders; the register does not tint rows.

### Inputs / Fields
- **Style:** surface fill, 1px line-strong border, 8px radius, 44px minimum height, 14px text; placeholders muted. Textareas start at 100px and resize vertically.
- **Focus:** the global 3px `--focus` outline at 3px offset — color is never the only focus signal.
- **Error:** the message sets `--failed-icon` at 14px; the field itself is not redrawn.
- **Checks:** 18px native boxes with the copper accent; **radio-group choices** render as bordered option cards with the selection state on selection.

### Navigation
- **Top bar:** 1px line border below, 44px-tall links in 15px muted; hover shifts to ink. The current route is ink at weight 600 with a 2px copper underline offset 8px (`aria-current="page"` drives it).
- **Ledger mark:** 40px (38px mobile) 40-unit SVG — an open book stroked in text color at 2.3/2px round joins, a center spine, three leader lines, and a copper bookmark stamp. The wordmark beside it is 29px at 650 with -0.035em tracking — the quiet, prominent identity the nav must keep.
- **Theme switch:** 7px-radius group with a 1px strong border; the pressed side fills with control-hover at weight 650, 50px minimum hit width.

### Workflow and Draft State
- **Setup:** discovery is explicit. One continuation control remains available with no selection, filtered/empty results, discovery errors, and partial-add errors; pending repository writes block continuation. Platform badges carry configuration status without a repeated subtitle.
- **Manual entry:** default to a known configured platform without replacing an operator's explicit selection. Unconfigured platforms remain selectable for preconfiguration; unknown status is not labeled unconfigured.
- **Global settings:** Save commits the draft and establishes its new baseline; Discard restores the saved values. Dirty route exits require confirmation and dirty unloads warn. An expired session preserves the in-memory draft for same-origin reauthentication; Discard clears the preservation notice. Draft credentials never gain a new browser-storage path.
- **Disclosure:** provider connection, model tuning, and additional commit types use native details. Every existing setting remains available; active uncommon/custom types remain discoverable, and invalid tuning fields are revealed and focused. Restoring the original effective values restores the clean state.
- **Mobile navigation:** Save/Discard, Form summary, and Back to Save and Discard links keep actions and effective settings reachable without leaving the draft. In-page hash links do not trigger the dirty-route confirmation.
- **404 recovery:** the primary action returns to Repositories rather than retrying the unavailable address. Preview/raw views and overwrite confirmation retain their publication safeguards.

### Accessibility and Motion
Skip link first, then `aria-current` on nav, `aria-pressed` on the theme switch, `role="status"` / `role="alert"` banners (error in the failed tint, warning in the cached tint), and labeled 44px controls. State transitions run 140ms ease — background, border, and color only; nothing moves. `prefers-reduced-motion: reduce` kills all animation, transitions, and smooth scrolling.

## Do's and Don'ts

### Do:
- **Do** keep every color in the named dual-plan token set (`.impeccable/design.json` carries both values); a new token exists only with a light and a dark face.
- **Do** hold the 44px gesture floor, 12px working surfaces, and the 20px/24px spacing rhythm.
- **Do** write result badges as the full quartet — hue-family text, tint, border, and 14px stroke icon — so meaning survives grayscale.
- **Do** let names and note text wrap; the mobile register keeps every column as a labeled row.
- **Do** reserve copper for the committed action and identity marks; let ink, muted, and tints do the quiet work.

### Don't:
- **Don't** bring back the Release Trace Wall — no Saira Stencil One, no powder-coated dark plates, no thermal-paper proof, no cyan trace, no heat signal.
- **Don't** paint tone settings, provider fields, or runtime copy in outcome colors; status colors belong to labeled states.
- **Don't** add marketing chrome to the workspace: hero, pitch lede, testimonial, activity graph, or health claim.
- **Don't** cast shadows on controls, chips, rows, or badges, or use elevation to signal state.
- **Don't** introduce a second display face, a gradient, glass, glow, or a third accent; one voice, one copper, two plans.
