---
target: /repos page
total_score: 22
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 3
timestamp: 2026-08-05T23-10-49Z
slug: web-src-routes-repos-page-svelte
---
## Design Health Score

| # | Heuristic | Score | Key Issue |
|---|-----------|-------|-----------|
| 1 | Visibility of System Status | 2 | Initial loading shown, but save/toggle/regenerate actions have no pending/busy state and rely on a single global error area. |
| 2 | Match Between System and Real World | 3 | Repo-management language is natural to operators; the native `window.prompt()` for regenerate feels out of place in a polished dashboard. |
| 3 | User Control and Freedom | 2 | No undo for toggles or saved settings; the only recovery is re-toggling and hoping the request succeeded. |
| 4 | Consistency and Standards | 3 | Buttons, chips, focus rings, and the table largely follow the Forged token system; active nav state is present. |
| 5 | Error Prevention | 2 | No confirmation before toggling, no inline validation on the settings form, and the tag prompt gives no format guidance. |
| 6 | Recognition Rather Than Recall | 3 | Effective tone/model are surfaced as chips, but per-repo settings are hidden inside an expandable row. |
| 7 | Flexibility and Efficiency of Use | 1 | No keyboard shortcuts, no bulk enable/disable, no search/filter, and no batch actions for power users. |
| 8 | Aesthetic and Minimalist Design | 3 | Flat, on-brand, and restrained; the empty-state/error juxtaposition creates noise. |
| 9 | Help Users Recognize, Diagnose, and Recover from Errors | 2 | Errors appear in plain text but are generic ("Request failed: 404 Not Found") with no recovery guidance. |
| 10 | Help and Documentation | 1 | No tooltips or inline help for tone, trigger, temperature, or instructions; first-timers must guess. |
| **Total** | | **22/40** | **Acceptable** |

## Design Specificity Verdict

The `/repos` page is clearly dressed in the Forged Release Notes world: forge-black ground, anvil-steel surfaces, heat-ramp committed-action buttons, Saira Stencil One title, and hairline borders. That specificity is strong at the surface level.

Where it weakens is in the interaction architecture. The list/table pattern is category-generic; it could host repositories, projects, or integrations from dozens of admin tools without changing shape. The empty state and settings row reveal no product point-of-view about what a "release-note source" is or why the operator should care. The visual identity is bespoke, but the experience still reads like a scaffolded CRUD table rather than a forge floor for shaping release notes.

## Overall Impression

A tasteful, on-brand dashboard surface that is undermined by operational gaps. The page looks correct but behaves tentatively: errors are dumped raw, async actions give no feedback, and important controls are not accessible. The biggest opportunity is to make the empty and editing states feel as considered as the landing page.

## What's Working

1. **Committed-action hierarchy is clear.** The "Add repositories" and "Save" buttons use the heat-ramp mark consistently, and the quieter "Refresh" / "Settings" actions stay visually subordinate.
2. **Empty-state card is well-placed.** A centered card with a single next step is the right pattern; it just needs to distinguish "no data" from "could not load data."
3. **Effective settings surfaced as chips.** Displaying the resolved tone and model in the table lets operators scan configuration at a glance without opening every row.

## Priority Issues

**[P1] Empty state conflates a load error with an actual empty repo list**
- **What:** When the backend cannot be reached, the page shows both a red "Request failed: 404 Not Found" and the "No repositories have been added yet" card.
- **Why it matters:** The operator receives two contradictory signals and no recovery path. They may waste time clicking "Add your first repository" when the real problem is connectivity.
- **Fix:** Differentiate the states: render a dedicated error panel with a retry action when loading fails, and show the empty state only when the request succeeds with an empty array.
- **Suggested command:** `$impeccable harden`

**[P1] Async actions have no pending or saving feedback**
- **What:** `saveSettings`, `toggleEnabled`, and `regenerate` do not disable buttons, show spinners, or indicate progress.
- **Why it matters:** Operators can double-submit, toggle repeatedly, or assume a click did nothing. The single global `error` area is too far from the action to be reliable feedback.
- **Fix:** Add per-row/per-action pending states: disable the button, swap its label to "Saving…" / "Generating…", and use live regions or inline alerts for success/failure.
- **Suggested command:** `$impeccable harden`

**[P1] Enable checkbox has no accessible name"
- **What:** Each row contains a native checkbox with `accent-mark` but no `<label>` or `aria-labelledby` tying it to the repository name.
- **Why it matters:** Screen-reader users hear only "checkbox checked" with no indication which repo is being enabled or disabled. The color-only status dot is similarly invisible to assistive tech.
- **Fix:** Wrap the checkbox in a `<label>` referencing `{r.owner}/{r.repo}`, or use `aria-label="Enable {owner}/{repo}"` and add visually hidden text for the dot.
- **Suggested command:** `$impeccable harden`

**[P2] Regenerate uses a native `window.prompt()`**
- **What:** Clicking "Regenerate" opens a browser prompt asking for a release tag.
- **Why it matters:** It blocks the main thread, cannot be styled, lacks validation or format hints, and breaks the immersive dashboard world.
- **Fix:** Replace the prompt with an inline input in the expanded row (or a small modal) that accepts the tag, validates the format, and provides context about the chosen repo.
- **Suggested command:** `$impeccable harden`

**[P2] Expanded settings are cramped on small screens and lack help**
- **What:** The settings form uses `sm:grid-cols-2`, so below 640 px it stacks, but the parent table already requires horizontal scroll on mobile. Fields for tone, trigger, model, and temperature have no explanation.
- **Why it matters:** First-time operators (Jordan) won't know what "temperature" does or the difference between "auto" and "manual" trigger. Mobile users (Casey) must pan and zoom to edit.
- **Fix:** Collapse to a single column earlier, add concise inline descriptions or tooltips for each field, and consider a side drawer instead of an expandable table row for editing.
- **Suggested command:** `$impeccable onboard`

## Persona Red Flags

**Alex (Power User)**
- No keyboard shortcuts for Refresh, Add, or Save.
- No bulk enable/disable; every repo must be toggled individually.
- No search or filter as the list grows.
- The `window.prompt()` for regenerate interrupts flow and prevents batch testing.

**Sam (Accessibility-Dependent User)**
- The enable checkbox has no accessible name.
- The settings button is not marked with `aria-expanded`; screen readers won't know the row opened.
- Async success/failure is not announced by a live region, so saving feels silent.
- Table horizontal scrolling is hard to navigate with a keyboard on narrow viewports.

**Jordan (Confused First-Timer)**
- "Request failed: 404 Not Found" is the only diagnostic when the backend is missing.
- Empty-state copy assumes success and offers no troubleshooting link.
- "Temperature," "trigger," and "instructions" have no inline help.
- After saving settings, there is no confirmation message.

## Minor Observations

- The global error message (`{error}`) is small and red but lacks an icon or retry association.
- Two similar calls to action appear when empty: the header "Add repositories" button and the card "Add your first repository" button.
- The table row hover uses top/bottom border color changes that are visually subtle and may be missed by low-vision users.
- Focus rings are present on most controls, which is good, but they are visually identical across all elements.

## Questions to Consider

- Should the per-repo settings live in an expandable table row, or would a side panel or modal make the relationship between repo and form clearer?
- What does "Enabled" mean to a first-time operator, and should the UI explain that it controls webhook auto-generation versus manual regeneration?
- Could a single bulk "Add/Enable" flow on this page remove the need to visit `/repos/add` repeatedly?
