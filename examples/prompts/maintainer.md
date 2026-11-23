Write release notes for maintainers, contributors, and technical readers who
work inside the codebase.

Lead with a short, direct statement of the headline changes. Name the
component, module, or behavior each concern touches, using the exact technical
terms the supplied source uses. Keep sentences short and factual. No marketing
language, no cheerleading, no vague summaries.

Keep every concern distinct:

- One primary bullet per concern, not per commit or PR. Prefer one short
  sentence and use at most two.
- Split unrelated changes from one source into separate concern bullets. Related
  commits may share a bullet only when all contributing sources are attributed.
- Never omit a selected distinct change, even one that looks minor, internal,
  or routine. Summarize internal helpers, implementation steps, tests, and review
  fixes unless the reader must act on them.
- Never invent a change, component, or behavior that is not present in the supplied source.

Preserve breaking, security, migration, changed-default, compatibility, and
required-action facts with all conditions and exceptions. Use a short nested
Required action: bullet for a migration the reader must perform or a step the
reader must take after upgrading. Use Caveat: only for a material condition,
exception, or limitation. State other required facts in the primary bullet. Do
not truncate the notes to a total length. When a supplied release-range net diff
establishes final behavior, prefer it to intermediate or squash-body inventories.
Do not present already-shipped, reverted, or superseded work as a new change.

State behavioral changes, changed defaults, configuration requirements, and
breaking changes in plain, direct language, with migration notes where relevant.
A fix names the defect and the component it affects. State performance and
reliability changes only when the supplied source supports an observable impact.

Do not add hashes or author names yourself. Keep the supplied source tokens
for Annalist to replace with verified links. Do not add details unsupported by
the selected messages and supplied diff.
