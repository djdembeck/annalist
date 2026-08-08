---
target: /setup page
total_score: 29
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 1
timestamp: 2026-08-05T22-09-20Z
slug: web-src-routes-setup-page-svelte
---
## Design Health Score

| # | Heuristic | Score | Key Issue |
|---|-----------|-------|-----------|
| 1 | Visibility of System Status | 3 | Stepper/buttons/aria-live present; "Loading…" text and disabled Continue without inline reason on Step 2. |
| 2 | Match System / Real World | 3 | Plain language and inline hints are strong; "voice" for tone is slightly abstract but clarified by examples. |
| 3 | User Control and Freedom | 3 | Completed steps clickable, skip flows confirmed, token toggled; no undo after add/save. |
| 4 | Consistency and Standards | 3 | Buttons/focus/borders consistent; Step 2 lacks the raised steel card used by steps 1, 3, 4, 5. |
| 5 | Error Prevention | 2 | Invalid primary actions disabled and skip warned; blank custom tone silently falls back. |
| 6 | Recognition Rather Than Recall | 4 | Options visible and labeled: token hint, platform status, repo names, voice descriptions, synthetic examples. |
| 7 | Flexibility and Efficiency of Use | 2 | Search/sort/filter/select-all help; no keyboard accelerators or bulk-import path. |
| 8 | Aesthetic and Minimalist Design | 3 | One heat action per step; Step 2 floating grid and expanding filter panel add modest noise. |
| 9 | Error Recovery | 3 | Errors in plain language; unauthorized redirect to Step 1 does not preserve token field. |
| 10 | Help and Documentation | 3 | Contextual hints and docs link present; no searchable help center (likely unnecessary here). |
| **Total** | | **29/40** | **Good, with room for refinement** |

## Design Specificity Verdict

The `/setup` page is clearly authored for Annalist's "Forged Release Notes" world rather than a generic onboarding wizard. The dark forge palette, heat-gradient committed-action buttons, Saira Stencil One display type, "forge" copy, and honest synthetic tone examples are product-specific. However, the underlying stepper/card structure is a familiar product-onboarding pattern, and Step 2 breaks the raised-steel-card rhythm used elsewhere, which slightly weakens the bespoke feel.

**Deterministic scan:** The CLI detector found zero anti-patterns in `web/src/routes/setup/+page.svelte` (exit 0). The browser overlay injected `detect.js` and reported one visual finding labeled "line length too long" (722 px wide overlay). This visual rule did not correspond to any semantic detector issue and is treated as a false positive.

## Overall Impression

The page is competent, on-brand, and trustworthy. Its biggest strengths are honesty ("Synthetic example" labels) and disciplined visual hierarchy. The most immediate payoff is fixing Step 2: a disabled primary action in a default-first-run state makes the product feel broken before the user has done anything wrong.

## What's Working

- **Honest synthetic voice examples** in Step 4 are explicitly labeled, building trust before the user commits to a tone.
- **The stepper** combines a heat underline for the active step with clickable completed steps, giving both orientation and control.
- **One committed heat action per step** is disciplined; the UI never competes for the primary action.

## Priority Issues

**[P1] Disabled Continue on Step 2 lacks an inline reason.** In the default first-run state, the primary call-to-action is grayed out, forcing users to guess why and hunt for the secondary "Skip for now" path. Add helper text below the button: "Connect at least one platform to continue, or skip below." Suggested command: `$impeccable clarify`.

**[P2] Step 2 is not wrapped in the shared raised steel card.** The exposed platform grid floats at a different elevation and breaks the "one raised steel card per step" rhythm. Wrap the Step 2 section in `rounded border border-line bg-surface-1 p-6`. Suggested command: `$impeccable layout`.

**[P2] Blank custom tone silently falls back to the server default.** Users selecting "Custom…" and leaving the field empty believe they set a custom tone, but `finishSetup` writes `null`. Disable "Finish setup" when `toneOption === 'custom'` and `customTone.trim()` is empty; show an inline hint. Suggested command: `$impeccable harden`.

**[P2] Repository loading and empty states are plain text.** "Loading…" and "No available GitHub repositories found." do not reassure first-time users or suggest what to do. Replace loading text with a skeleton list and the empty state with actionable guidance about token scopes/hidden filters. Suggested command: `$impeccable onboard`.

**[P3] Platform "Not configured" status uses a red alert icon.** Red reads as an error, but an unconfigured platform is the expected first-run state. Use a neutral `ink-3` status icon/dot and reserve red for actual errors. Suggested command: `$impeccable colorize`.

## Persona Red Flags

**Jordan (First-Time Operator)**
- The disabled Continue button on Step 2 gives no explanation, so they don't know why they can't proceed.
- The red X next to an unconfigured platform makes the default first-run state feel like a failure.
- The `ADMIN_TOKEN` label and `config.yaml` hint assume they already know where the server's environment file lives.

**Alex (Power Admin)**
- No bulk-import path for repositories; they must scroll and checkbox a potentially large list.
- Selecting "Custom…" and leaving the field blank silently writes `null`, discarding intent.
- Unauthorized redirect to `/setup` wipes the token field instead of preserving it for correction.

**Sam (Keyboard/Screen-Reader User)**
- The Show/Hide token toggle is a `<button>` nested inside the `ADMIN_TOKEN` `<label>`, creating a confusing label-within-label interaction.
- Disabled primary buttons are removed from the tab order, so they may not discover them or their context.

## Minor Observations

- Step 2 platform cards use `opacity-90` when unconfigured, which slightly mutes text contrast for an informational state.
- The Step 1 Continue button changes to "Connecting…" but shows no spinner, making the busy state less obvious.
- Step 5's "Note preview" label uses `uppercase tracking-wide`, which reads as a brand-world kicker; acceptable because it labels a preview section, not a heading.
- The repo list uses `max-h-80 overflow-auto` inside a card; on small viewports the internal scrollbar may be hard to discover.

## Questions to Consider

- Why is Step 2 the only step not wrapped in the shared raised steel card, and does that inconsistency signal a different kind of task to the user?
- What if the Done summary surfaced masked platform names and a token identifier instead of only counts, so operators could verify the configuration before leaving setup?
- Could the voice preview be shown by default for the current selection rather than requiring the user to change the dropdown first?
