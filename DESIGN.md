---
name: Annalist
description: Self-hosted AI release notes for GitHub and Forgejo — The Copy Desk dashboard.
colors:
  page: "#0d0f13"
  surface-1: "#131722"
  surface-2: "#1a1f2c"
  line: "#232a38"
  line-strong: "#2d3547"
  focus: "#8a93a6"
  ink: "#eef1f6"
  ink-2: "#b9c0cd"
  ink-3: "#6f7788"
  control: "#2b3342"
  control-hover: "#37415a"
  mark: "#d65a4f"
  mark-hover: "#e0695d"
  ok: "#3fb477"
  alert: "#e05252"
typography:
  title:
    fontFamily: "ui-sans-serif, system-ui, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 700
    lineHeight: 1.2
  headline:
    fontFamily: "ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 600
    lineHeight: 1.5
  body:
    fontFamily: "ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "ui-sans-serif, system-ui, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 400
    lineHeight: 1.4
rounded:
  sm: "4px"
  full: "9999px"
spacing:
  "1": "4px"
  "2": "8px"
  "3": "12px"
  "4": "16px"
  "6": "24px"
components:
  button-primary:
    backgroundColor: "{colors.control}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "8px 12px"
  button-primary-hover:
    backgroundColor: "{colors.control-hover}"
  button-mark:
    backgroundColor: "{colors.mark}"
    textColor: "{colors.page}"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "8px 16px"
  button-mark-hover:
    backgroundColor: "{colors.mark-hover}"
  button-secondary:
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "6px 12px"
  button-secondary-hover:
    backgroundColor: "{colors.control}"
  field:
    backgroundColor: "{colors.surface-1}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "8px 12px"
  card:
    backgroundColor: "{colors.surface-1}"
    textColor: "{colors.ink-2}"
    rounded: "{rounded.sm}"
    padding: "16px"
  chip:
    backgroundColor: "{colors.surface-2}"
    textColor: "{colors.ink-2}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "2px 8px"
---

# Design System: Annalist

## Overview

**Creative North Star: "The Copy Desk"**

Annalist's dashboard is the desk where a release's voice is set before it ships. Like a newsroom's copy desk — where prose is shaped, checked, and released without ceremony — the interface is dark, calm, and entirely in service of the work: repositories, settings, and the machine that writes notes while everyone sleeps. There is no decoration, no chrome, no theater; every pixel either carries information or gives the operator room to breathe.

The system is **editorial restraint** rendered in graphite. It is monochrome by conviction: cool-lean charcoal surfaces on a near-black page, a cold-press paper text ramp, and exactly one proofing color — a desaturated vermilion used the way an editor uses a pen, for the single committed action and little else. Hierarchy arrives through weight, size, and tonal stepping, never through color or shadow. The dashboard is flat, quiet, and legible at a glance, the way an instrument panel should be.

**Key Characteristics:**
- Dark-first, cool-lean graphite palette; near-black page (`#0d0f13`) with one- and two-step raised surfaces.
- Color is signal: vermilion for the committed action, green/red strictly for platform status, nothing decorative.
- Flat by default — depth comes from 1px hairline borders and tonal steps, never shadows.
- Precise, restrained controls: 4px radius, weight-differentiated states, hover = one tonal step lighter.
- System font stack; typography does hierarchy by weight and size, not by face.

## Colors

One sentence: cool-lean midnight graphite surfaces under cold-press paper ink, with a single vermilion proofing mark and green/red reserved for signal.

### Primary
- **Editor's Vermilion** (`#d65a4f`, hover `#e0695d`): the single accent. Used for the committed action on a surface (Setup → Continue, Settings → Save) and as the checkbox accent fill. It is the red pen — it makes a mark, then stays off the page.

### Neutral
- **Midnight Graphite** (`#0d0f13`): page background. The whole system sits on it.
- **Carbon Slate** (`#131722`): first raised surface — nav bar, cards, table headers, input/select/textarea fills. The work-surface tone.
- **Toned Iron** (`#1a1f2c`): second raised step — chips, secondary buttons, inset controls. 60% opacity variant backs expandable table rows.
- **Soft Graphite** (`#2b3342`) → **Lifted Graphite** (`#37415a`): neutral button fill and its hover. Quiet controls that never compete with the mark.
- **Hairline Steel** (`#232a38`): 1px borders and row dividers.
- **Field Edge** (`#2d3547`): input borders — one step brighter than dividers so fields read as editable.
- **Correction Steel** (`#8a93a6`): focus border on focused fields.
- **Cold Press Ink** (`#eef1f6`): primary text and button labels.
- **Draft Gray** (`#b9c0cd`): secondary text — field labels, table secondary cells, subtitles.
- **Footnote Gray** (`#6f7788`): muted hints, loading/empty states, ghost captions.
- **Proofread Green** (`#3fb477`): configured / saved status.
- **Red-Pen Red** (`#e05252`): errors and misconfigured status; shares the vermilion hue family, but is signal, not decoration.

### Named Rules
**The Editor's Mark Rule.** The vermilion appears only for the committed action on a surface (Setup Continue, Settings Save), checkbox accent, and nothing else. Two vermilion elements on one screen is a copy error.

**The Signal-Only Color Rule.** Green and red mean status, never decoration. A page is monochrome until a platform reports configured, saved, or failed.

**The Ink-On-Mark Rule.** Text on vermilion buttons is Midnight Graphite (`text-page`), because the mark is a working surface, not a bright badge.

## Typography

**Display/Body Font:** `ui-sans-serif, system-ui, sans-serif` — the system stack. No webfonts, no display face. The dashboard is an instrument; the type gets out of the way.

**Character:** Operative and quiet. All hierarchy is achieved with weight, size, and color stepping within one neutral sans stack — the same discipline the rest of the system follows.

### Hierarchy
- **Page Title** (bold 700, 24px / `text-2xl`, line-height 1.2): the H1 on each surface ("Repositories", "Settings", "Setup") in Cold Press Ink.
- **Section Heading** (semibold 600, 14px / `text-sm`, line-height 1.5): H2s like "LLM endpoint" and setup guidance, in Cold Press Ink.
- **Body / Field Label** (regular 400, 14px / `text-sm`, line-height 1.5): labels and values. Labels render in Draft Gray; primary values in Cold Press Ink.
- **Label / Caption** (regular 400, 12px / `text-xs`, line-height 1.4): chips, field mini-labels, "Effective" summaries, footnote captions — Draft Gray or Footnote Gray.
- **Mono:** note-preview output uses the browser default monospace (`<pre>`); not system-controlled.

### Named Rules
**The One-Face Rule.** No display type, no webfonts. If a hierarchy problem needs a new size or weight, use the existing stack. A second typeface is a redesign, not a tweak.

## Layout

- **App shell:** full-width top nav with a 1px Hairline Steel bottom border; content column `max-w-5xl` (Repositories) or centered `max-w-2xl` (Setup, Settings) with 24px padding.
- **Spacing rhythm:** Tailwind's 4px base scale — 4 / 8 / 12 / 16 / 24px (gap-1 … gap-6). Dense but not cramped; the calm comes from consistent increments.
- **Responsive:** grids are `grid gap-4` collapsing to one column below 640px (`sm:grid-cols-2`); tables are full-width inside an `overflow-x-auto` scroller so narrow viewports scroll rather than break.
- Tables: 16px horizontal cell padding, 8px vertical; header row distinct via Carbon Slate fill.

## Elevation & Depth

Flat by default. There are **no box-shadows** in the system. Depth is conveyed three ways: 1px Hairline Steel borders that separate surfaces; tonal stepping from page → Carbon Slate → Toned Iron; and a `black/40` scrim behind the note-preview `<pre>` to drop it back from the desk. Anything that needs lifting does it by one tonal step, never by shadow.

## Shapes

Uniform precision: **4px radius** (`rounded`) on every control, card, chip, table, and input — buttons, selects, textareas, the note preview. The only exception is the `rounded-full` 10px status dot (configured / misconfigured indicators). No clipping, no pill buttons, no new geometry. The radius is a constant because the system's identity is repetition, not variation.

## Components

### Buttons
- **Shape:** 4px radius, flat, no shadow.
- **Quiet Primary** (`button-primary`) — row and page actions (Refresh, Settings, Save on a repo row): Soft Graphite fill, Cold Press Ink label, 8px padding; hover lifts one step to Lifted Graphite. 500 weight on larger labels.
- **Committed Primary** (`button-mark`) — the one action that ships something (Setup Continue, Settings Save): Editor's Vermilion fill, Midnight Graphite label (`text-page`), 8px 16px padding; hover uses the lighter mark. Reserve for exactly one per surface.
- **Secondary** (`button-secondary`) — Regenerate: Toned Iron fill, Ink label; hover to Soft Graphite.
- **States:** hover is always one tonal step; no brightness tricks, no glow.

### Checkboxes
- 16px native checkbox with vermilion accent (`accent-mark`); used for enable/force-regenerate toggles.

### Chips
- Toned Iron fill, Draft Gray 12px text, 4px radius, 2px 8px padding. Read-only value tags (effective tone/model); no states.

### Cards / Containers
- Carbon Slate fill, Hairline Steel 1px border, 4px radius, 16px padding, flat. Used for status cards, the LLM endpoint panel, and the "Effective" readings.

### Inputs / Fields
- Carbon Slate fill, Field Edge 1px border, 4px radius, 8px 12px padding, Cold Press Ink text.
- **Focus:** border shifts to Correction Steel; no glow, no ring — focus is a line change.
- Selects render with dark options by default; textareas share the same field style.

### Navigation
- Carbon Slate bar, full width, Hairline Steel bottom border; wordmark "Annalist" 16px semibold in Cold Press Ink; links (Setup, Repos, Settings) 14px Draft Gray hover → Cold Press Ink. No active-state styling — the current surface's H1 is the position marker.

### Tables
- Header row: Carbon Slate fill, Draft Gray medium weight; body rows Hairline Steel top borders; cells 16/8px padding; the expandable per-repo settings row drops one tone step (`Carbon Slate / 60`) to sit under its row.

### Note Preview
- Rendered generated notes in `<pre>`: `black/40` scrim, Hairline Steel border, 12px Draft Gray mono, `max-h-64` scroll.

## Do's and Don'ts

### Do:
- **Do** keep the page on Midnight Graphite and raise surfaces only one or two tonal steps (Carbon Slate, Toned Iron).
- **Do** use the vermilion mark for exactly one committed action per surface, plus the checkbox accent.
- **Do** treat green/red strictly as status signal (config dots, Saved, errors).
- **Do** prefer tonal separation and hairlines over shadows — flat is the system.
- **Do** keep the 4px radius constant across controls, cards, chips, and inputs.
- **Do** show hierarchy with weight, size, and tonal stepping within the system font stack.

### Don't:
- **Don't** introduce a second accent hue (teal, blue, purple) — the palette is graphite + vermilion + signal green/red.
- **Don't** add box-shadows, gradients, or glass effects to lift anything.
- **Don't** fall back to default Tailwind zinc classes; the bespoke graphite ramp is the only allowed neutral.
- **Don't** use the vermilion as link color, decoration, or ambient fill — its rarity is the point.
- **Don't** use white text on vermilion buttons; mark buttons carry Midnight Graphite labels.
- **Don't** change the radius per component; variation in shape reads as error.
