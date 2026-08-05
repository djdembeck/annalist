---
version: 1
slug: "route-setup"
primary_target: "route:/setup"
related_targets: ["route:/repos/add","route:/settings"]
---

# Surface: Setup (`/setup`)

## Mode and job

**Operate.** First-run onboarding: authenticate with `ADMIN_TOKEN`, verify platform configuration, add repositories, choose a default voice, and confirm readiness before release events can flow.

## Visitor

A developer or operator who just deployed Annalist and needs to configure the instance before leaving it unattended.

## Task and states

1. **Authenticate** — paste `ADMIN_TOKEN` and connect.
2. **Connect platforms** — review whether GitHub and/or Forgejo are configured; skip only after explicit warning.
3. **Add repositories** — load available repos from connected platforms, filter, select, and add.
4. **Choose voice** — pick `inherit`, a built-in preset (`chronicler`, `engineer`, `launch`), or a custom persona.
5. **Done** — summary of token, platforms, repos, and tone with onward links.

Key states: empty/invalid token, no platforms configured, loading repo list, empty repo list, skipped platform setup.

## Direction

Inherits the **Forged Release Notes** world. The stepper is the only persistent chrome; each step is one raised steel card with a single heat-gradient committed action. Completed steps recede to muted ink; the active step carries the heat underline.

## Constraints

- No configured platform must not block progress; skipping must be explicit and warned.
- Synthetic tone examples must be labeled honestly.
- One heat-mark action per step; green/red reserved strictly for status.
