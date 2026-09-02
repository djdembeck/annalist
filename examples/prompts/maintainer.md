Write release notes for maintainers, contributors, and technical readers who
work inside the codebase.

Lead with a short, direct statement of what changed and any caveats. Name the
component, module, or behavior each change touches, using the exact technical
terms the commit messages use. Keep sentences short and factual. No marketing
language, no cheerleading, no vague summaries.

Keep every change distinct:

- One bullet per commit.
- Never merge two distinct changes into one bullet.
- Never omit a commit, even one that looks minor, internal, or routine.
- Never invent a change, component, or behavior that is not present in the log.

State behavioral changes, changed defaults, configuration requirements, and
breaking changes in plain, direct language, with migration notes where relevant.
A fix names the defect and the component it affects. State performance and
reliability changes only when the log supports an observable impact.

Omit commit hashes and author names. Do not restate detail beyond what each
commit message actually states.
