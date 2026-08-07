---
version: 1
slug: "route"
primary_target: "route:/"
related_targets: ["route:/setup"]
---

# Surface Brief: Public Landing (`/`)

## Mode and job

**Persuade.** Explain what Annalist does, show the release path as a readable proof, and give a self-hosting operator a direct next step without inventing customers, metrics, or activity.

## Visitor

A developer or operator shipping releases on GitHub or Forgejo/Gitea who wants consistent, human-sounding release notes without hand-writing them. The visitor is evaluating a self-hosted product and needs mechanism and truthful deployment details before acting.

## Route composition

The public route stays inside the shared Release Trace Wall shell and operator navigation. Its first viewport is a two-part work surface on wide screens: a left editorial block with the headline, GitHub/Forgejo purpose, `Get started` link to `/setup`, GitHub link, and a self-hosted proof caption; a right synthetic trace panel.

The trace panel is the memorable proof, not a claim of live activity. It is labeled `Synthetic release trace / v1.4.0` and contains:

- four synthetic commit subjects as the input workpiece;
- a readable `Receive → Resolve → Strike` rail, with a cyan signal showing the active stage;
- a thermal-paper `Shaped note proof` containing synthetic output; and
- a caption stating that real releases use the operator's configured tone and repository history.

Below the first viewport, the route keeps the same dark powder-coated panels and trace language:

1. A four-panel capability row covers cross-platform operation, voice control, self-hosting, and zero-touch behavior.
2. A three-panel voice section shows `chronicler`, `engineer`, and `launch` note samples, each marked `Synthetic`.
3. A deployment section pairs copyable `docker pull ghcr.io/djdembeck/annalist:latest` text with the examples-pack link and the documented environment-variable names.
4. The footer keeps GitHub, Examples, Get started, and the UNLICENSED label as plain links/metadata.

## States and interaction

- The trace loops only as explanatory motion; the note remains readable at rest. Browser visibility pauses it, and reduced motion settles it on the completed state.
- The Docker action changes from `Copy` to `Copied` for two seconds when clipboard access succeeds; unsupported clipboard contexts do not fabricate a success message.
- Synthetic output is labeled at the trace and tone-example surfaces. No customer, usage, delivery, or performance numbers appear.
- Primary action is copper; secondary deployment/GitHub actions use raised controls. Cyan is reserved for active trace and focus, while green/red remain status semantics.

## Responsive and accessibility requirements

At 768px and below, the hero and deployment columns stack; at 639px and below, feature panels and full-width hero actions stack. Long commands and note output scroll or wrap without clipping. Keep the semantic heading structure, descriptive link labels, focus-visible rings, live copy feedback, and `prefers-reduced-motion` behavior intact.

## Related surfaces

- `/setup` — first-run activation and configuration.
- `/repos` — authenticated repository management after setup.
