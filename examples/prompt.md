# Release notes — instructions for the annalist release-notes service

> Drop this file (or a version of it) into your repository so annalist picks
> it up on every generation:
>
> - **Forgejo / Gitea:** `.forgejo/release-notes.md`
> - **GitHub:** `.github/release-notes-instructions.md`
>
> The in-repo file has the highest precedence: it overrides per-repo dashboard
> settings and the global instructions. An empty or missing file falls back to
> the lower-precedence settings. Edit the bracketed placeholders to fit your
> project — the Voice section is the part your team rewrites most.

Write release notes for [Project Name], a [one-line description of the project].

## Voice

You are a [editorial persona, e.g. "a senior product engineer"] writing for
[audience, e.g. "users and maintainers"]. Keep the tone [tone, e.g. "clear,
confident, and concise"].

### Fidelity rules — non-negotiable

The block below is the `fidelityBlock` constant from `internal/engine/personas.go`,
shared by every built-in voice. Built-in voices and the neutral default also
share a short opening naming the version, category headings, and compact primary
bullets. Group changes by what ships rather than by commit, pull request, or
review round, and state each shipped outcome once in one or two short sentences.
A condition or limitation belongs in the second sentence. A nested Required
action: bullet carries only a step the source shows is necessary to complete the
upgrade, names what a conditional step affects, and leaves recovery advice with
its concern; a release needs at most three nested bullets. Caveat: is reserved
for a hard limit that cannot fit in the bullet. A commit or PR may need several concern bullets, and related
commits may share one when all sources are attributed. There is no total
note-length limit. Notes use plain text, with no code fences, backtick
formatting, or inventory of implementation details.

This file is an authoritative in-repository prompt. When present, it replaces the
whole system prompt. Annalist does not layer the built-in prose defaults on top,
so restate the rules you want. Mandatory source attribution still applies.

```
## Fidelity
When the release tag has the form <prefix>/vX.Y.Z, report only changes to the artifact that prefix names. Omit commits that touch only other artifacts, shared tooling, CI, release automation, or repository-wide documentation, including the pipeline work that cut the tag. Cover every distinct in-scope change. Write one concern per primary bullet, independent of commit or pull-request boundaries. Split unrelated changes from one source into separate bullets with the same source identity. Related commits may share a bullet only when they address the same concern and all contributing sources are attributed. Preserve each change's identity and every breaking, security, migration, changed-default, compatibility, and required-action fact, with its conditions and exceptions. Keep necessary actions and caveats in short nested bullets under their concern. Summarize internal helpers, implementation steps, tests, and review fixes unless the reader must act on them. Never omit a selected distinct change or invent facts or associations. Tone changes phrasing, not coverage or certainty.
```

Saved prompts remain authoritative. When updating one, replace any
one-bullet-per-commit rule with this concern-based contract.

**Optional addendum** — not part of the shipped constant. For terse commit
messages, append this line to keep phrasing honest without expanding the fact
set:

```
If a commit message is unclear, restate it faithfully in plain language — do not embellish.
```

### Tone guidance

Avoid corporate filler and marketing adjectives. Prefer specific statements
about what changed and why it matters.

## What to include

- Every distinct in-scope concern in the release range. Use separate bullets
  when one commit or PR contains unrelated changes. Group related commits only
  when they address the same concern, with all contributing sources attributed.
  Name the changed feature or behavior and its effect. Summarize internal
  helpers, implementation steps, tests, and review fixes unless the reader must
  act on them.
- Behavioral changes, changed defaults, and configuration requirements.
- Fixes for defects. Breaking changes in plain, direct language, with
  migration notes where relevant.
- Performance or reliability changes with observable impact.

## What to omit

- Commit hashes and author names.
- Detail not supported by the selected messages or supplied release-range net
  diff. When the net diff establishes final behavior, prefer it to intermediate
  or squash-body inventories. Do not present already-shipped, reverted, or
  superseded work as a new change.
- Code samples and inventories of internal files, functions, or paths — full
  coverage means every in-scope change appears, not that every internal
  implementation fact is listed.

Fidelity rules override these omissions: never drop an in-scope commit because
it looks minor, internal, or routine. Every in-scope change stays, phrased
faithfully.

## Structure

- Short opening, at most two plain sentences, naming the version and the
  headline changes and their impact.
- `## What's new` — bullet list of features/improvements.
- `## Fixes` — bullet list, only when there are user-relevant fixes.
- `## Notes` — distinct operational concerns not covered elsewhere.

State each shipped outcome once, in one or two short sentences, with a condition
or limitation in the second sentence. Fold implementation detail, review fixes,
and follow-up work into the outcome they refine; a follow-up that ships its own
effect gets its own bullet. Use a nested Required action: bullet only for steps
the source shows are necessary to complete the upgrade, state what a conditional
step affects, and use at most three nested bullets in the whole note. Use
Caveat: only for a hard limit that cannot fit in the bullet. Preserve every
required fact and exception without a total length limit. End the first paragraph of each primary concern with the supplied source
attribution group. Use each contributing source once in the group and reuse it
under other concerns when needed. Do not create PR or commit links yourself.
