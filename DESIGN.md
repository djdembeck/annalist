---
name: Annalist
description: Self-hosted AI release notes for GitHub and Forgejo — the Forged Release Notes dashboard.
colors:
  page: "#0a0a0c"
  surface-1: "#14161a"
  surface-2: "#1b1e22"
  line: "#2a2e35"
  line-strong: "#3a3f48"
  focus: "#948e84"
  ink: "#f3f0ec"
  ink-2: "#b8b2a8"
  ink-3: "#8a847b"
  control: "#23262b"
  control-hover: "#2d3138"
  mark: "#ff7b00"
  mark-hover: "#ffb300"
  cherry: "#ff3d00"
  ember: "#ff7b00"
  heat: "#ffb300"
  white: "#fff7e8"
  ok: "#3fb477"
  alert: "#e05252"
typography:
  title:
    fontFamily: '"Saira Stencil One", ui-sans-serif, system-ui, sans-serif'
    fontSize: "1.5rem"
    fontWeight: 400
    lineHeight: 1.2
  headline:
    fontFamily: '"Saira", ui-sans-serif, system-ui, sans-serif'
    fontSize: "0.875rem"
    fontWeight: 600
    lineHeight: 1.5
  body:
    fontFamily: '"Saira", ui-sans-serif, system-ui, sans-serif'
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: '"Saira", ui-sans-serif, system-ui, sans-serif'
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
    backgroundColor: "#23262b"
    textColor: "#f3f0ec"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "8px 12px"
  button-primary-hover:
    backgroundColor: "#2d3138"
  button-mark:
    backgroundColor: "linear-gradient(90deg, #ff3d00, #ff7b00, #ffb300)"
    textColor: "#0a0a0c"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "8px 16px"
  button-mark-hover:
    backgroundColor: "linear-gradient(90deg, #ff7b00, #ffb300, #fff7e8)"
  button-secondary:
    backgroundColor: "#1b1e22"
    textColor: "#f3f0ec"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "6px 12px"
  button-secondary-hover:
    backgroundColor: "#23262b"
  field:
    backgroundColor: "#14161a"
    textColor: "#f3f0ec"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "8px 12px"
  card:
    backgroundColor: "#14161a"
    textColor: "#b8b2a8"
    rounded: "{rounded.sm}"
    padding: "16px"
  chip:
    backgroundColor: "#1b1e22"
    textColor: "#b8b2a8"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "2px 8px"
---

# Design System: Annalist

## Overview

**Creative North Star: "Forged Release Notes"**

Annalist is the forge where raw commits are heated, shaped by settings, and struck into human-sounding release notes. The interface is dark, warm, and industrial: forge-black ground, anvil-steel panels, and a heat ramp from cherry to white-hot for the committed action. Every surface — public landing page and operator dashboard alike — shares this world. There is no chrome, no decoration; every element either carries information or gives the operator room to work.

**Key Characteristics:**
- Dark-first forge palette; near-black page (`#0a0a0c`) with one- and two-step raised steel surfaces.
- Heat is signal: the cherry-to-ember-to-heat gradient marks the single committed action. Green/red remain strictly for status.
- Flat by default — depth comes from 1px hairline borders and tonal steps, never shadows.
- One stenciled display face (`Saira Stencil One`) for titles, wordmark, and measured labels; `Saira` for body and UI. Self-hosted.
- Uniform 4px radius on every control, card, chip, table, and input.

## Colors

One sentence: forge-black ground under warm paper ink, with an anvil-steel surface ramp and a single heat-ramp accent used like a striking mark.

### Primary
- **Heat Mark** (`#ff7b00`, hover `#ffb300`): the single accent. Rendered as a horizontal gradient from **Cherry** (`#ff3d00`) through **Ember** (`#ff7b00`) to **Heat** (`#ffb300`). Used for the committed action on a surface and the checkbox accent fill. It is the strike — it makes a mark, then stays off the page.

### Neutral
- **Forge Black** (`#0a0a0c`): page background.
- **Anvil Steel** (`#14161a`): first raised surface — nav bar, cards, table headers, input/select/textarea fills.
- **Pitted Steel** (`#1b1e22`): second raised step — chips, secondary buttons, inset controls. 60% opacity variant backs expandable table rows.
- **Tool Steel** (`#23262b`) → **Worked Steel** (`#2d3138`): neutral button fill and its hover.
- **Hairline Scale** (`#2a2e35`): 1px borders and row dividers.
- **Field Edge** (`#3a3f48`): input borders — one step brighter than dividers so fields read as editable.
- **Focus Haze** (`#948e84`): focus border on focused fields.
- **Cold-Press Ink** (`#f3f0ec`): primary text and button labels.
- **Draft Ash** (`#b8b2a8`): secondary text — field labels, table secondary cells, subtitles.
- **Slag Gray** (`#8a847b`): muted hints, loading/empty states, ghost captions.
- **White Hot** (`#fff7e8`): display headline text and the peak of the heat ramp.

### Status
- **Forged Green** (`#3fb477`): configured / saved status.
- **Scrap Red** (`#e05252`): errors and misconfigured status.

### Named Rules
**The Heat-Mark Rule.** The heat gradient appears only for the committed action on a surface and the checkbox accent. Two heat-ramp elements on one screen is a copy error.

**The Signal-Only Color Rule.** Green and red mean status, never decoration.

**The Ink-On-Mark Rule.** Text on mark buttons is Forge Black (`text-page`) because the mark is a working surface.

## Typography

**Display Font:** `Saira Stencil One`, self-hosted. Used for page titles, the wordmark, numeric step labels, and small measured panel labels.

**Body/UI Font:** `Saira`, self-hosted. Weights 400–600. Tabular numerals for metrics.

**Code Font:** browser monospace stack (`ui-monospace`, SFMono, Menlo, Consolas).

**Character:** Operative and warm. Hierarchy is achieved with the stencil display face for position, weight and size within Saira for detail, and tonal stepping for priority.

### Hierarchy
- **Page Title** (`font-display`, `text-2xl`, White Hot): the H1 on each surface ("REPOSITORIES", "SETTINGS", "SETUP").
- **Section Heading** (`font-sans` semibold, `text-sm`): H2s like "LLM endpoint" and setup guidance, in Cold-Press Ink.
- **Body / Field Label** (`font-sans` regular, `text-sm`): labels and values. Labels in Draft Ash; primary values in Cold-Press Ink.
- **Caption / Metric** (`font-sans` regular, `text-xs`, tabular numerals): chips, field mini-labels, "Effective" summaries, footnote captions — Draft Ash or Slag Gray.
- **Mono:** code blocks, Docker commands, generated release notes.

## Layout

- **App shell:** full-width top nav with 1px Hairline Scale bottom border; content column `max-w-5xl` (Repositories, Add repositories) or centered `max-w-3xl` (Setup, Settings) with 24px padding.
- **Spacing rhythm:** Tailwind's 4px base scale — 4 / 8 / 12 / 16 / 24px.
- **Responsive:** grids collapse to one column below 640px; tables scroll horizontally inside `overflow-x-auto`.
- Tables: 16px horizontal cell padding, 12px vertical; header row distinct via Anvil Steel fill.

## Elevation & Depth

Flat by default. No box-shadows. Depth is conveyed by 1px Hairline Scale borders and tonal stepping from Forge Black → Anvil Steel → Pitted Steel.

## Shapes

Uniform 4px radius (`rounded`) on every control, card, chip, table, and input. The only exception is the `rounded-full` status dot. No clipping, no pill buttons.

## Components

### Buttons
- **Quiet Primary** (`button-primary`) — page actions (Refresh, Settings row): Tool Steel fill, Cold-Press Ink label; hover to Worked Steel.
- **Committed Primary** (`button-mark`) — the action that ships something (Continue, Save, Add repositories): heat-gradient fill, Forge Black label, 4px radius, `brightness-110` hover. Reserve for the single committed action per surface.
- **Secondary** (`button-secondary`) — Regenerate: Pitted Steel fill, Ink label; hover to Tool Steel.

### Checkboxes
Native checkbox with heat accent (`accent-mark`).

### Chips
Pitted Steel fill, Draft Ash 12px text, 4px radius, 2px 8px padding. Read-only value tags (effective tone/model).

### Cards / Containers
Anvil Steel fill, Hairline Scale 1px border, 4px radius, 16px padding, flat.

### Inputs / Fields
Anvil Steel fill, Field Edge 1px border, 4px radius, 8px 12px padding, Cold-Press Ink text.
- **Focus:** border shifts to Focus Haze; no glow.

### Navigation
Anvil Steel bar, full width, Hairline Scale bottom border; wordmark "ANNALIST" in `Saira Stencil One`; links 14px Draft Ash hover → Cold-Press Ink. No active-state styling.

### Tables
Header row: Anvil Steel fill, Draft Ash medium weight; body rows Hairline Scale top borders; expandable per-repo settings row drops one tone step (`bg-surface-1/60`).

### Note Preview
Generated notes in `<pre>`: Forge Black background, Hairline Scale border, 12px Slag Gray mono, `max-h-64` scroll.

## Onboarding

The `/repos` page lists only repositories the operator has deliberately added. New users are guided to `/repos/add`, where they can:
- Browse repositories available from each enabled platform (GitHub / Forgejo).
- Select multiple available repos and add them at once.
- Add a repository manually by platform, owner, and name.

Empty states explain the next step rather than assuming misconfiguration. Managed repos can be enabled/disabled, tuned per-repo, and regenerated from the list.

## Do's and Don'ts

### Do:
- Keep the page on Forge Black and raise surfaces only one or two tonal steps.
- Use the heat-ramp mark for exactly one committed action per surface, plus the checkbox accent.
- Treat green/red strictly as status signal.
- Prefer tonal separation and hairlines over shadows.
- Keep the 4px radius constant across controls, cards, chips, and inputs.
- Use the display face for position and the UI face for detail.

### Don't:
- Don't introduce a second accent hue (teal, blue, purple) alongside the heat ramp.
- Don't add box-shadows, gradients, or glass effects as decoration.
- Don't fall back to default Tailwind zinc classes; the bespoke forge ramp is the only allowed neutral.
- Don't use the heat-ramp as link color, decoration, or ambient fill.
- Don't use white text on mark buttons; mark buttons carry Forge Black labels.
- Don't change the radius per component.

---

# Surface: Public Landing Page ("/")

## Mode and World

Public landing page. Mode: **Persuade**. It dramatizes the pipeline — raw commits are heated, each summary is a measured blow, and a human-sounding release note exits as shaped metal. It shares the same Forged world as the dashboard, but is allowed to be more expressive because its only job is to explain and drive deployment.

## Palette, Typography, Components

Uses the global Forged system. Dedicated accents:
- Large display headline in `Saira Stencil One`, White Hot.
- Hero "forge stage" card with an animated heat bar and a static, honestly-labeled synthetic release note.
- Three "blow rows" explain the pipeline as Receive → Resolve → Strike.
- Deploy section presents a copyable Docker command.

## Motion

One controlled animation: commits and heat values animate through the forge-stage bar in a loop; the synthetic output below is always readable so the proof is never hidden. Respects `prefers-reduced-motion`.
