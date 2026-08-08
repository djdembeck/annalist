---
version: 1
slug: "route"
primary_target: "route:/"
related_targets: ["route:/setup"]
---

# Surface Brief: Installed Home (`/`)

## Mode and job

**Operate.** Help an operator configure an installed Annalist instance, understand the release-to-note sequence, and reach the first-run setup without reading a marketing pitch.

## Visitor

A developer or operator who has already deployed Annalist and needs to make this instance useful. They need the setup order, the inputs to have nearby, the behavior after setup, and a plain preview of tone choices.

## Route composition

The home opens as a setup station inside the shared Release Trace Wall shell:

- a direct headline and `Open setup` action;
- a four-stage `After setup` release path showing release, webhook, repository history, and note output;
- a three-row setup sequence for admin access, repositories, and tone;
- a short prerequisite list for the admin token, forge access, and LLM endpoint;
- a rendered release proof using the same trace component as the pipeline; and
- one readable tone preview with chronicler, engineer, and launch tabs rather than three cramped note windows.

Marketing claims, customer proof, deployment copy, and long feature sections do not belong in the installed home. The GitHub Pages product page should be a separate static surface so the dashboard remains task-oriented.

## States and interaction

- `Open setup` is the primary action and all setup rows preserve a direct path into `/setup`.
- The release trace loops only as explanatory motion; reduced motion settles it and browser visibility pauses it.
- Tone tabs update one rendered Markdown preview. The preview is illustrative and never claims live repository activity.
- The release proof and tone preview use bounded scroll regions and wrapping content so headings and sections remain readable at narrow widths.

## Responsive and accessibility requirements

At 760px and below, setup rows place their action under the copy and the prerequisite/tone columns stack. At 639px and below, hero actions become full-width and the setup sequence remains readable without horizontal scrolling. Preserve semantic heading order, tab roles and selection state, descriptive links, visible focus rings, and reduced-motion behavior.

## Related surfaces

- `/setup` — first-run activation and configuration.
- `/repos` — authenticated repository management after setup.
- `/settings` — global tone and model defaults after setup.
