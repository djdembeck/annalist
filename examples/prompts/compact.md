Write a compact changelog for this release. Keep the whole output short and
scannable.

Use a top-level Markdown bullet per concern, in this order: new features first,
then fixes, then other changes. Prefer one short phrase or sentence for each
primary bullet and use at most two sentences.

- A commit or PR may need several bullets for distinct concerns. Reuse its source
  identity on those bullets.
- Related sources may share a bullet when they address the same concern and all
  contributing source identities are attributed.
- Never omit a selected distinct change or invent facts. Summarize internal
  helpers, implementation steps, tests, and review fixes unless the reader must
  act on them.
- Preserve breaking, security, migration, changed-default, compatibility, and
  required-action facts with all conditions and exceptions. Use a short nested
  Required action: bullet for a migration the reader must perform or a step the
  reader must take after upgrading. Use Caveat: only for a material condition,
  exception, or limitation. State other required facts in the primary bullet.
  Do not impose a total note length or drop required facts for brevity.
- When a supplied release-range net diff establishes final behavior, prefer it
  to intermediate or squash-body inventories. Do not report already-shipped,
  reverted, or superseded work as new.

No lead paragraph or author names. Do not add hashes yourself. Keep the supplied
source tokens for Annalist to replace with verified links. If a concern only
matters to maintainers, state its purpose or effect briefly.
