---
version: 1
slug: "route-setup"
primary_target: "route:/setup"
related_targets: ["route:/repos/add", "route:/settings"]
---

# Surface: Setup (`/setup`)

## Mode and job

**Operate.** First-run activation lets an operator authenticate with `ADMIN_TOKEN`, inspect platform readiness, choose repositories, set the default note voice, and confirm what will happen before release events flow.

## Visitor

A developer or operator who has deployed Annalist and is configuring the self-hosted instance for unattended release-note generation on GitHub and/or Forgejo/Gitea.

## Route composition

Setup is a Release Trace Wall workspace, not a separate onboarding theme. A section header names `FIRST-RUN ACTIVATION`, explains the goal, and shows the current station count. On desktop, a left `RELEASE TRACE` progress panel sits beside a right station workspace; at 680px and below the progress panel and workspace stack.

The five stations are:

1. **Welcome & Token — `COMMIT INPUT`:** password-style `ADMIN_TOKEN` field with show/hide control, a copper Continue action, and a busy message while the token is checked.
2. **Connect platforms — `SIGNAL SOURCES`:** GitHub and Forgejo cards report Connected/Offline from actual configuration. Unconfigured sources link to platform setup instructions. Continue is available when a source is configured; `Skip for now` exposes an explicit warning path when none is connected.
3. **Add repositories — `REPOSITORY WORKPIECE`:** load available repositories, switch between enabled sources, search, optionally sort by name/activity, reveal filters for forks and shared namespaces, select rows, and add the selected repositories. The route explains that Annalist reads release webhooks and commit history and writes only to the release body.
4. **Choose voice — `NOTE SHAPING`:** choose `inherit`, `chronicler`, `engineer`, `launch`, or `custom`. Presets show a labeled synthetic note preview; custom tone requires text before Finish setup can commit it.
5. **Done — `RELEASE READY`:** summarize token, connected platform count, repositories added, and default tone; show a labeled synthetic note proof; link to `/repos` and `/settings`.

The left trace keeps completed stations available as buttons, marks the current station with cyan active treatment, and leaves future stations queued. The station header reports checking, repository loading, settings writing, or ready state without hiding the active work surface.

## States and interaction

- Empty/invalid token: the Continue action is disabled without input; rejected credentials appear as an assertive error and do not advance the station.
- Platform readiness: no configured source is a valid state, but continuing requires an explicit warning and `Continue anyway`; no repository list is implied.
- Repository intake: loading uses visible skeleton rows; an empty filtered list explains how to clear search or enable namespaces/forks; unauthorized responses return to setup.
- Voice: the inherit explanation, preset synthetic examples, and custom-tone validation are distinct states. Saving announces writing progress and resets when complete.
- Done: summary values reflect the current setup session; the note proof remains explicitly synthetic and the onward links are ordinary navigation.

Copper is reserved for the station's committing action. Cyan marks active trace/focus, heat supports active work cues, and green/red communicate only connected/complete versus invalid/error/skipped status.

## Responsive and accessibility requirements

Use the existing semantic form labels, native controls, `aria-current="step"`, `aria-live` station/loading updates, and `role="alert"`/`role="status"` for errors and warnings. Every field and action retains a visible cyan focus ring. Buttons remain at least 44px high; repository names and note proofs wrap or scroll instead of clipping. Reduced motion shortens transitions and stops any active animation without removing status text or proof.

## Related surfaces

- `/repos/add` — repository intake for operators who skip or revisit setup.
- `/settings` — global voice and machine contract after activation.
