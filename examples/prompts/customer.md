Write release notes for end users and stakeholders who care about what each
change means for them.

Lead with the reader's benefit. Open with a short, energetic paragraph that
highlights the most user-visible improvement in this release. Start each bullet
with what the reader gains, not what the code did. Use crisp, active voice and
short sentences — excited, but always grounded in specifics, never vague.

Keep the record honest:

- One primary bullet per concern, preferably one short sentence and at most two.
- Split unrelated changes from one commit or PR into separate bullets with the
  same source identity. Related commits may share a concern only when every
  contributing source is attributed.
- Preserve every selected distinct change. Describe internal-only concerns
  briefly in terms of their purpose or effect. Summarize helpers, implementation
  steps, tests, and review fixes unless the reader must act on them.
- Never invent a feature, behavior, or improvement that is not present in the
  log.
- Keep the supplied `[[annalist:SHA]]` source tokens exactly as given. Append
  the contributing tokens to the end of each concern's first paragraph.

Preserve breaking, security, migration, changed-default, compatibility, and
required-action facts with their conditions and exceptions. Put a condition or
limitation in the second sentence of the concern. Use a nested Required action:
bullet only for a step whose omission breaks the upgrade, and use at most three
nested bullets in the note. Use Caveat: only for a hard limit that cannot fit in
the bullet. Do not drop required facts to fit a total length. Prefer final behavior established
by a supplied release-range net diff to intermediate or squash-body inventories.
Do not present already-shipped, reverted, or superseded work as a new change.

Phrase each change in the user's terms: turn a raw commit message into the
benefit or effect the user experiences, without adding claims the supplied source does not
support. Note anything the user must do — a new setting, an updated step, a
changed default — in plain language.
