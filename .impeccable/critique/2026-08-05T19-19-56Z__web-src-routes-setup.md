---
target: /setup page
total_score: 29
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 2
timestamp: 2026-08-05T19-19-56Z
slug: web-src-routes-setup
---
#### Report header provenance

Method: dual-agent (A: AssessmentAv2 · B: AssessmentBv2). Browser overlay was attempted but could not reliably inject `detect.js` because the browser worker repeatedly failed during request-interception teardown, so CLI detector output is the authoritative mechanical evidence.

#### Design Health Score

| # | Heuristic | Score | Key Issue |
|---|-----------|-------|-----------|
| 1 | Visibility of System Status | 3 | Stepper orients; busy states labeled; token errors clear. Gap: no inline success confirmation after repo add, count only on Done. |
| 2 | Match System / Real World | 3 | Operator language is correct, but preset codenames (`chronicler`/`engineer`/`launch`) and `inherit` require translation. |
| 3 | User Control and Freedom | 3 | Click-back to completed steps, explicit skip warnings. No undo for mis-added repo; no search clear affordance. |
| 4 | Consistency and Standards | 3 | One heat action, one secondary treatment throughout. Three different skip labels for the same action. |
| 5 | Error Prevention | 3 | Disabled continue/add, explicit skip warning, safe `inherit` default. No token reveal/verify toggle. |
| 6 | Recognition Rather Than Recall | 3 | Options are mostly visible; Done recap helps. Preset labels and GitHub App procedure demand recall. |
| 7 | Flexibility and Efficiency of Use | 2 | Select-all + bulk Add and step jumpback exist, but no keyboard shortcuts, no fast repo list import, extra "Load repositories" click. |
| 8 | Aesthetic and Minimalist Design | 3 | Clean card hierarchy; voice step is text-dense with three full examples + sample commits + dropdown. |
| 9 | Error Recovery | 3 | Token error is plain and actionable; state preserved. Non-auth errors may surface raw `e.message`. |
| 10 | Help and Documentation | 3 | Inline platform steps and honest examples are strong. No pointer to ADMIN_TOKEN source or external docs. |
| **Total** | | **29/40** | **Good** |

#### Design Specificity Verdict

**LLM assessment:** High specificity. The page is authored for the Forged Release Notes world rather than a category template. The cherry→ember→heat gradient is reserved for committed actions and the active step, anvil-steel surfaces and hairline borders give the wizard a physical forged feel, and Saira Stencil One + Saira create a consistent typographic identity. The only generic scaffolding is the numbered stepper itself, which reads as a conventional ledger rather than a forge artifact, and the Done step misses a chance to dramatize "your forge is ready" with a first-note preview.

**Deterministic scan:** Clean. `detect.mjs` scanned `web/src/routes/setup/+page.svelte`, `web/src/routes/+layout.svelte`, and `web/src/app.css` and returned zero findings across every rule. A control sample with known anti-patterns produced findings, confirming the detector is functioning.

**Visual overlays:** No reliable browser overlay is available. The browser worker repeatedly errored on request-interception teardown during `detect.js` injection and screenshot capture, so the CLI scan is the only verified mechanical signal. No user-visible overlay exists in a browser tab.

#### Overall Impression

A solid, on-brand onboarding flow that respects the Forged Release Notes aesthetic. The biggest opportunity is to stop treating external platform setup as an in-wizard task: step 2 drops the user into a GitHub App creation chore that neither fits a linear wizard nor can be completed inside the app. Step 3 also turns a wizard step into a full repo-management mini-app, overwhelming the single decision the user came to make.

#### What's Working

1. **Relentless brand consistency.** The heat-ramp committed-action, steel surfaces, stencil display face, and warm ink ramp make the flow feel authored for Annalist rather than a stock wizard.
2. **The stepper is real orientation, not decoration.** Heat underline shows position, completed steps show checkmarks, and the user can jump back to any reached step.
3. **Honest scaffolding.** Synthetic voice examples are clearly labeled, platform instructions are inline, the read-only permission model is stated plainly, and skipping requires an explicit warning.

#### Priority Issues

**[P1] What:** Step 2 embeds a full manual GitHub/Forgejo App/webhook setup procedure inside a linear wizard.  
**Why it matters:** The user cannot complete this in-session. It forces a multi-tab context switch, creates a memory bridge, and is the most likely abandonment point.  
**Fix:** Collapse step 2 to a status check (Configured / Not configured per source) with a link to `/settings` or docs for the full procedure; preserve the skip-warning.  
**Suggested command:** `$impeccable layout`

**[P1] What:** Step 3 "Add repositories" exposes six controls at once: search, sort, two checkbox filters, source tabs, and select-all, plus the repo list and Add button.  
**Why it matters:** Exceeds the ≤4-option working-memory guideline; incidental filters compete with the essential decision of selecting repos.  
**Fix:** Prominently show search + repo list only. Move sort/fork/shared-namespace toggles behind a "Show filters" disclosure.  
**Suggested command:** `$impeccable distill`

**[P2] What:** The `ADMIN_TOKEN` input is a masked password field with no reveal/verify toggle and no pointer to where the token comes from.  
**Why it matters:** Silent paste errors (trailing spaces, partial copies) produce a confusing unauthorized failure; first-time deployers don't know the token origin.  
**Fix:** Add a show/hide toggle and a small "Where do I find this?" hint linking to env-var / deploy docs.  
**Suggested command:** `$impeccable clarify`

**[P2] What:** Voice presets are exposed as internal codenames (`chronicler`, `engineer`, `launch`) and `inherit` is unexplained; step 4 also stacks three full synthetic examples + sample commits.  
**Why it matters:** The user must translate labels before choosing; the step is text-heavy for one low-stakes decision.  
**Fix:** Use human labels with one-line previews, explain `inherit` in a clause, and trim to a single side-by-side example comparison.  
**Suggested command:** `$impeccable clarify`

**[P3] What:** The Done step is a flat summary card with two links and no "what happens next" orientation.  
**Why it matters:** This is the peak-end moment; a bare recap undersells the payoff and leaves first-timers without a clear next action.  
**Fix:** Add a one-line "what happens next" and a taste of the product (e.g., an example generated note now that tone is chosen).  
**Suggested command:** `$impeccable delight`

#### Persona Red Flags

**Jordan (First-Timer):**
- Step 2 demands a GitHub App creation procedure mid-onboarding with no docs callback — the most likely abandonment point.
- Voice preset names (`chronicler`/`engineer`/`launch`) and `inherit` are vague until selected and read.
- No guidance about where `ADMIN_TOKEN` comes from at the token field.

**Alex (Power User):**
- No keyboard shortcuts; token paste still needs a click and repos require an extra "Load repositories" click.
- No fast bulk path beyond select-all — no paste-a-list or CLI import for many repos.
- No inline success confirmation after adding repos; counts only appear on Done.

**Sam (Accessibility):**
- Step transitions and busy states are not announced via `aria-live` (only errors live inside the region).
- "Not configured" uses red alert styling for a neutral/unset condition — color-coded as an error.
- Muted `ink-3` labels on near-black surfaces risk low contrast, and the token field has no reveal toggle.

#### Minor Observations

- Add button grammar: "Add 1 selected" for a single repo.
- "Not configured" red X reads alarmist for a neutral state.
- Three skip labels express the same action: "Skip setup for now", "Skip for now", "Skip — I'll add repositories later".
- Nav keeps "Setup" active after setup completes; no chrome-level "done" state.
- Step 4's sample-commits box is redundant with preset previews.
- The stepper's completed check uses green `ok` for a neutral progress mark, edging outside the "green/red reserved strictly for status" rule.

#### Questions to Consider

1. Should platform credential setup live inside a linear wizard at all, or should `/setup` collect only connection status and route the actual configuration to `/settings` + docs?
2. What would make step 2 feel like progress rather than a chore — e.g., a tickable readiness checklist for external GitHub/Forgejo steps?
3. Does the voice step need three full-length synthetic examples, or would one honest side-by-side comparison say more with less?
4. What should the Done state actually promise a first-timer — can we show a first generated release-note preview to prove "your forge is ready"?
