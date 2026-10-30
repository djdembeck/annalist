---
version: 1
slug: "route-repos"
primary_target: "route:/repos"
related_targets: ["route:/setup","route:/repos/add","route:/settings","route:/"]
---

# Repositories and release monitoring

Mode: Operate. Scope: `/repos` with the shared shell, `/repos/add`, `/settings`, `/setup` and entry redirect. Operators inspect real server generation outcomes, change repository automation, generate or preview notes, and configure writing/provider settings. Configuration is not a health claim. History contains accepted daemon attempts only; empty history is explicit.

## Direction contract

THESIS: A warm working interface that makes release outcomes and automation configuration easy to compare; no marketing hero, trace/station costume, or ornamental dashboard metrics.

OWN-WORLD: Warm paper canvas and white work plane in light mode; tobacco charcoal and cream in dark mode. Humanist system UI text, copper actions, 12px working surfaces, and quiet top navigation with one visible Gather lockup selected by CSS from two flat images (light: copper symbol with ink wordmark; dark: lightened copper symbol with cream wordmark, shown only under `data-theme="dark"` because WebKit does not inherit `color-scheme` into embedded SVG media queries): the symbol is two entry bars and a continuous returning contour whose bottom arm is the third entry, paired with an outlined neutral Annalist wordmark. Result badges combine high-contrast text, restrained tint, and distinct icons. Use the full lockup in nav/README and the standalone Gather symbol for favicons, app icons, and avatars, never action buttons or outcome badges; do not split-color the symbol or make the wordmark copper.

STORY: Identify the repository, read its latest observed outcome, distinguish configured triggers, inspect a failed or preserved run, then adjust settings or generate notes without losing context.

FIRST VIEWPORT: At 1440px, a 1200px centered shell; one visible `/annalist-logo-light.svg` image (intrinsic 984×256), paired in the same link with a CSS-hidden `/annalist-logo-dark.svg` twin that `data-theme="dark"` swaps in, displayed 48px high in an Annalist-home-labeled link with decorative empty alt, navigation, and theme control above a Repositories heading with Refresh/Add actions, followed by aligned Repository / Latest generation / Automation / Actions columns and inline run details. The lockup is 44px high at ≤980px and 40px at ≤360px; mobile retains all information in labeled rows, wrapping names at the namespace slash.

FORM: User-approved Quiet Workspace world with Standards Reference information density; the built `/repos` route is the composition authority. seed:c2f9040bf910e851. The user pinned this combination after the direction round.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
