---
name: Annalist
description: Self-hosted AI release-note automation for GitHub and Forgejo/Gitea — the Release Trace Wall.
colors:
  page: "#0b0f14"
  panel: "#18212b"
  raised: "#202d38"
  panel-warm: "#1b2831"
  raised-warm: "#25343e"
  line: "#33434f"
  line-strong: "#4a5d68"
  focus: "#59c3c3"
  ink: "#f3eee5"
  muted: "#aeb8bd"
  ink-soft: "#96a4aa"
  control: "#263640"
  control-hover: "#30444f"
  copper: "#f06a3a"
  copper-hover: "#f58b5f"
  heat: "#f4c95d"
  cyan: "#59c3c3"
  green: "#86b574"
  red: "#d55c51"
  paper-edge: "#c2c0b8"
typography:
  display:
    fontFamily: '"Saira Stencil One", "Saira", ui-sans-serif, system-ui, sans-serif'
    fontSize: "clamp(2.75rem, 7vw, 5.75rem)"
    fontWeight: 400
    lineHeight: 0.95
    letterSpacing: "-0.035em"
  headline:
    fontFamily: '"Saira", ui-sans-serif, system-ui, sans-serif'
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: 1.3
  body:
    fontFamily: '"Saira", ui-sans-serif, system-ui, sans-serif'
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "ui-monospace, SFMono-Regular, \"SF Mono\", Menlo, Consolas, monospace"
    fontSize: "0.6875rem"
    fontWeight: 600
    lineHeight: 1.2
    letterSpacing: "0.1em"
  mono:
    fontFamily: "ui-monospace, SFMono-Regular, \"SF Mono\", Menlo, Consolas, monospace"
    fontSize: "0.75rem"
    fontWeight: 400
    lineHeight: 1.5
rounded:
  paper: "2px"
  sm: "4px"
  full: "9999px"
spacing:
  "1": "4px"
  "2": "8px"
  "3": "12px"
  "4": "16px"
  "5": "20px"
  "6": "24px"
components:
  button-primary:
    backgroundColor: "{colors.copper}"
    textColor: "{colors.page}"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "8px 12px"
    height: "44px"
  button-primary-hover:
    backgroundColor: "{colors.copper-hover}"
  button-secondary:
    backgroundColor: "{colors.raised}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "8px 12px"
    height: "44px"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.muted}"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "4px 8px"
  field:
    backgroundColor: "{colors.panel}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.sm}"
    padding: "8px 12px"
    height: "44px"
  panel:
    backgroundColor: "{colors.panel}"
    textColor: "{colors.ink}"
    rounded: "{rounded.sm}"
    padding: "16px"
  chip:
    backgroundColor: "{colors.raised}"
    textColor: "{colors.muted}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "4px 8px"
  note-paper:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.page}"
    rounded: "{rounded.paper}"
    padding: "16px"
---

# Design System: Annalist

## Overview

**Creative North Star: "Release Trace Wall"**

Annalist presents each release as a traceable workpiece moving from input to proof. Dark powder-coated panels hold the operator's work; a thermal-paper note proof makes the generated result tangible. Copper marks the committed action, cyan shows the active trace, and compact instrumentation labels keep the system legible without turning the dashboard into decoration. The installed home uses the same wall to explain setup and the release path, while authenticated routes let an operator configure it.

The system is deliberately layered rather than glossy. Saira Stencil One establishes position for the wordmark and display headings; Saira carries the working copy; monospace labels and note output read like instrumentation. A 4px control language, hairline borders, and tonal planes create depth without shadows. Green and red are reserved for status so their meaning stays unambiguous.

**Key Characteristics:**
- Dark powder-coated page and panel surfaces with one raised tonal step.
- Copper is the committed action; cyan is the active trace; heat is a supporting work signal.
- Thermal-paper note proof is the light, readable output plane.
- Saira Stencil One for display; Saira for UI copy; monospace for labels and generated notes.
- Flat depth: hairline borders and tonal planes, never drop shadows.

## Colors

The palette is a dark blue-black wall with cool paper ink, copper action, cyan trace, and tightly scoped status signals.

### Primary
- **Copper:** The committed action on a surface. It is direct and solid, not a decorative gradient.

### Secondary
- **Trace Cyan:** Active rails, nodes, focus rings, selected navigation, and live trace state.
- **Working Heat:** Received commit signals, active setup indicators, and other in-progress work cues; it is not a success or failure status.

### Neutral
- **Page:** The dark wall behind the shell.
- **Panel:** The powder-coated base plane for navigation, cards, and fields.
- **Raised:** The one-step-up plane for controls, chips, trace nodes, and row hover.
- **Panel Warm / Raised Warm:** Optional warm tonal planes where a surface needs a slight material shift.
- **Line / Line Strong:** Hairline dividers and stronger field/control edges.
- **Ink / Muted / Ink Soft:** Primary copy, secondary copy, and quiet captions.
- **Control / Control Hover:** Neutral control fill and its one-step hover state.
- **Paper Edge:** The border around the thermal-paper proof.

### Status
- **Green:** Configured, saved, complete, or connected state only.
- **Red:** Error, invalid input, or explicitly skipped/misconfigured state only.

### Named Rules
**The Copper Action Rule.** Use copper for the action that commits the current work; do not turn it into ambient decoration or a second primary on the same surface.

**The Signal-Only Color Rule.** Cyan, heat, green, and red each carry a defined operational signal. Green and red never decorate a neutral surface.

## Typography

**Display Font:** `Saira Stencil One`, self-hosted, with Saira and system fallbacks.
**Body/UI Font:** `Saira`, self-hosted, with a system fallback.
**Label/Mono Font:** `ui-monospace`, SFMono, Menlo, Consolas, or the platform equivalent.

**Character:** The display face gives the wall a measured technical identity without competing with the work. Saira keeps instructions and controls plain; uppercase monospace labels make station, trace, and status metadata easy to scan.

### Hierarchy
- **Display:** Saira Stencil One, regular 400, responsive hero size (2.75–5.75rem), tight 0.95 line-height; landing headline and major surface identity.
- **Headline:** Saira, semibold 600, approximately 1rem with 1.3 line-height; section and panel headings.
- **Title:** Saira Stencil One, regular 400, approximately 1.05rem in the shell brand; compact route identity.
- **Body:** Saira, regular 400, 0.875rem with 1.5 line-height; instructions, labels, and values.
- **Label:** Monospace, semibold 600, 0.6875rem, uppercase with tracked lettering; trace labels, station labels, and compact status metadata.
- **Proof / Code:** Monospace at compact reading sizes; Docker commands, commit subjects, and generated note output.

### Named Rules
**The Instrument-Label Rule.** Use tracked monospace labels for metadata and state; never use them to replace readable headings or instructions.

## Layout

The shared shell uses a sticky full-width operator navigation bar and a centered working column. Console routes use a 64rem maximum; the home route uses a 6xl wall. A 12-column console grid and a 4px-based rhythm (4, 8, 12, 16, 20, and 24px) keep panels dense but readable. The installed home opens with setup guidance beside a four-stage release path, then moves through rendered proof and tone control.

At mobile widths, the navigation wraps, console grids become one column, the home setup path stacks its release-flow panel below the headline, and trace rails/workspaces stack vertically. Setup's progress rail moves above its station workspace at 680px; repository, intake, and settings rails stack at 760px. Horizontal lists remain scrollable where preserving a row is useful. Keep controls at the existing 44px minimum and let long repository names and note text wrap rather than clip.

## Elevation & Depth

The system has no box shadows. Powder-coated panels sit on the page by tonal separation and 1px borders; raised controls and trace nodes use the next tonal plane. The thermal-paper proof intentionally flips to an ink-on-paper plane so generated notes are easy to read, with a fine paper edge rather than a glow.

### Shadow Vocabulary
- **None:** `box-shadow: none`; use the page, panel, raised, and paper planes plus hairlines for depth.

### Named Rules
**The Tonal-Plane Rule.** Add depth with a border or the next named surface tone, never with a shadow, glass treatment, or decorative glow.

## Shapes

Controls, panels, chips, trace nodes, and navigation use a restrained 4px radius. Status and signal dots are circular; the thermal-paper proof uses a smaller 2px radius. Borders are explicit and generally 1px. There are no pill-shaped controls or clipped panels.

## Components

### Buttons
- **Primary:** Copper fill with page-colored text, 44px minimum height, 4px radius; hover lightens to copper-hover. Use for the current committed action.
- **Secondary:** Raised fill with ink text and a strong line; hover shifts to the control plane.
- **Ghost / Link:** Transparent at rest with muted text; hover adds a raised plane and line. Links retain visible focus and never borrow copper as decoration.
- **Disabled:** Reduced opacity and not-allowed cursor; disabled controls do not communicate success or failure through color.

### Chips and Status
- **Chip:** Raised plane, muted monospace label, 4px radius, compact padding; used for counts, preview labels, and station metadata.
- **Status:** Monospace uppercase text with a current-color border. Green and red communicate status; cyan/heat communicate active work, never success/failure.

### Cards / Panels
- **Panel:** Panel plane, 1px line border, 4px radius, 16px padding; `panel-soft` uses the page plane for an inset workspace.
- **Trace rail:** A hairline track with circular nodes. Cyan marks active/current progress; the rail remains readable as plain text and list structure.
- **Note proof:** Ink-colored thermal-paper plane, dark page-colored text, paper edge, 2px radius, scrollable monospace output.

### Inputs / Fields
- **Style:** Panel fill, strong line border, 4px radius, 44px minimum height, ink text, muted placeholder.
- **Focus:** Cyan border plus a visible 2px outline; do not rely on color alone to identify the focused field.
- **Error / Disabled:** Red is reserved for an error message or invalid state; disabled fields use opacity and cursor treatment.

### Navigation

The sticky shell bar uses a panel plane and hairline bottom border. The ANNALIST wordmark uses the display face; links use Saira and a 4px control shape. The current route gains ink/cyan text, a raised plane, and a short cyan underline. Public and setup remain reachable without an authenticated token; other console routes redirect to setup when no token is present.

### Accessibility and Motion

Use semantic headings, labeled form controls, native buttons/inputs, and `aria-current`, `aria-live`, and `role="status"`/`role="alert"` for changing station, loading, and error states. All interactive elements expose a 2px cyan `:focus-visible` ring with offset. The release trace labels its commit subjects and output as an example; `prefers-reduced-motion` settles the trace and reduces transitions, while visibility pauses the looping signal.

## Do's and Don'ts

### Do:
- **Do** use the named page, panel, raised, line, ink, copper, heat, cyan, green, and red roles rather than one-off colors.
- **Do** make the trace legible in text and structure before adding motion; label example proof clearly.
- **Do** use copper for the current committed action and cyan for active trace/focus.
- **Do** preserve the thermal-paper proof as a high-contrast output plane with readable monospace text.
- **Do** use borders and tonal planes for depth and keep the 4px control rhythm.
- **Do** preserve focus rings, live status announcements, wrapping text, and reduced-motion behavior.

### Don't:
- **Don't** revive the old Copy Desk or Forged Release Notes visual language.
- **Don't** use gradients, glass, glow, or shadows as decorative depth.
- **Don't** use green or red for emphasis, links, or decoration; they are status semantics.
- **Don't** use cyan or heat as a generic accent or substitute for readable status text.
- **Don't** fabricate customers, metrics, release activity, or operational outcomes in public proof.
- **Don't** create a second radius system or hide long names and note output by clipping.
