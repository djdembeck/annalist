# Release Notes — Voices and Fidelity

## 1. What makes a good changelog

A changelog records what changed in a release, one concern per entry. It is not a marketing newsletter, a commit log, or a patch diff — it is a structured, readable record of user-relevant changes.

**Core principles (sourced from [Keep a Changelog 2.0.0](https://keepachangelog.com/en/2.0.0/)):**

- Entries belong in one of six types: **Added**, **Changed**, **Deprecated**, **Removed**, **Fixed**, **Security**.
- Each entry addresses a single concern. Do not combine unrelated changes into one bullet.
- Group entries by type. Do not include empty type sections.
- Date or version-tag each release.
- The changelog replaces the raw git log as the canonical record of what changed.

**Additional fidelity guidance (sourced from [LiteLLM's RELEASE_NOTES_GENERATION_INSTRUCTIONS](https://github.com/BerriAI/litellm/blob/main/cookbook/misc/RELEASE_NOTES_GENERATION_INSTRUCTIONS.md)):**

- Categorize each change before writing it.
- Preserve the factual content of every commit — the identity of each change, and every breaking, security, and migration fact, whether or not the reader must act. Tone and phrasing are presentation layers, not filters on the facts. Covering every change fully does not mean inventorying every implementation detail behind it.

## 2. Why LLM release notes lose fidelity

Large and small language models share three failure modes when generating changelogs:

1. **Omission.** The model drops commits it considers "unimportant." This is the most common error. A dependency bump or internal refactor that the model discards may have been the only commit in the release range. The model's importance filter is not the author's.

2. **Concern bundling.** A model treats a commit or PR as the size of a bullet, even when a squash body contains several unrelated changes. The result is a paragraph that mixes features, fixes, and caveats. The reverse also loses facts: unrelated changes from separate commits can collapse into one vague summary. Bullets follow concerns, while attribution tracks every contributing source.

3. **Editorialization and invention.** The model adds adjectives, claims, or behaviors not supported by the commit log. "Significantly improved performance" appears when the commits say nothing about benchmarks. "Fixed a long-standing bug" appears when the commit says "fix null check."

The root cause is that models are trained to be helpful summaries, not faithful records. Without an explicit contract separating "what changed" from "how you phrase it," the model optimizes for readability over accuracy.

## 3. The fidelity contract

Every built-in voice (`chronicler`, `engineer`, `launch`, and the neutral
default) shares one output shape and fidelity contract. Start with one plain
sentence for a small release, at most two for a large release, naming the version
and headline changes. Follow with `## category` headings and one primary bullet
per concern, preferably one short sentence and at most two.

A commit or PR can contain several distinct changes. Give each concern its own
bullet and reuse that source identity. Related commits may share a bullet when
they address the same concern and all contributing sources are attributed.
Preserve every selected distinct change. Summarize internal helpers,
implementation steps, tests, and review fixes unless the reader must act on them.
A distinct internal change still needs a concise account of its purpose or effect.

Keep breaking, security, migration, changed-default, compatibility, and
required-action facts with their conditions and exceptions. Group changes by
what ships, not by commit, pull request, or review round, and state each shipped
outcome once. State a condition, exception, or limitation in the second sentence
of the primary bullet. Use a nested Required action: bullet only for steps the
source shows are necessary to complete the upgrade, and state who or what a
conditional step affects before the instruction. Keep recovery advice for
individual failures with its concern, outside the label. A release needs at most
three nested bullets in total. Use Caveat: only for a hard limit or data-loss
risk that cannot fit in the bullet. Do not drop required facts to fit a total
note length. Notes use plain text: no code fences, backtick formatting, or inventory
of internal files and functions. Necessary public interface, configuration, and
version names remain exact. The **Breaking:** marker remains exact.

Summarize required actions and prerequisites when the full procedure belongs in
documentation. Include a version-appropriate guide link only when the source
provides one. Do not invent URLs or breaking-change markers. Keep the source's
certainty exact. When a supplied release-range net diff establishes final
behavior, prefer it to intermediate or squash-body inventories. Already-shipped,
reverted, or superseded work must not appear as a new change. Limited diff
context does not excuse dropping required facts from commit bodies.

The block below is the `## Fidelity` section shared verbatim by every built-in
persona. The shared rules restate its scope and concern-based coverage.
Authoritative full prompts (an in-repository instructions file or a named
profile's prompt) replace the whole system prompt and bypass these defaults.
A custom freeform `tone` still receives the shared rules, but not this block as
its own section.

```
## Fidelity
When the release tag has the form <prefix>/vX.Y.Z, report only changes to the artifact that prefix names. Omit commits that touch only other artifacts, shared tooling, CI, release automation, or repository-wide documentation, including the pipeline work that cut the tag. Cover every distinct in-scope change. Group changes by what ships, not by commit, pull request, or review round. Use one primary bullet for each distinct shipped outcome or distinct internal change. Summarize internal helpers, implementation steps, tests, and review fixes that contribute to the same outcome in that bullet. Preserve any separate shipped effect, including one introduced by a follow-up. Attribute every contributing source to the concern it supports. If a source supports separate concerns, cite it in each. Preserve each change's identity and every breaking, security, migration, changed-default, compatibility, and required-action fact, with its conditions and exceptions. Keep those facts with the concern they qualify, whether or not they warrant a nested label. Never omit a selected distinct change or invent facts or associations. Tone changes phrasing, not coverage or certainty.
```

Saved prompts remain authoritative. When updating one, replace any
one-bullet-per-commit rule with this concern-based contract.

**What full coverage means.** Every selected distinct change appears, and every
in-scope source identity is accounted for. A unique object-name prefix of at
least eight characters names the same source as its full token. Source counts
and bullet counts need
not match. One source may support several concerns, and several related sources
may support one concern. Required facts stay with their concern. Internal
implementation and review details do not need a line-by-line inventory.

**Optional addendum** — not part of the shipped `fidelityBlock`. If your commit
messages are terse, append this line below the block to keep phrasing honest
without expanding the fact set:

```
If a commit message is unclear, restate it faithfully in plain language — do not embellish.
```

This contract applies whether the prompt carries only `<commit_log>` or also
a `<diff>` block. Source collection removes an empty repeat only when Git
metadata proves there is no tree change and its full message duplicates a
retained entry. Distinct empty commits and nonempty changes with a shared
subject remain. Output bullets are never deduplicated after generation.

### Automatic source citations

Every voice and profile receives the same mandatory citation contract in
Annalist's generated user message. Custom full prompts keep their system text
unchanged. Each primary concern must be a top-level Markdown list item. End its
first paragraph with the supplied source tokens for every contributing
commit, separated by spaces on one line. A source appears once within a group
and may recur in other concern items. Keep multiline qualifiers, commands, and
nested required actions inside their concern. Tokens cannot appear in
introductions, headings, code, or nested actions. Do not wrap changes in HTML
blocks or comments.

For prefixed tags, out-of-artifact commits remain omitted and need no item.
Coverage is not checked for those tags, so an in-range omission in a scoped
release publishes without a gate failure. For unprefixed tags, every selected
source must be represented, whether alone, grouped with related sources, or
repeated across distinct concerns.

Annalist resolves those exact commit SHAs through authenticated platform APIs
and renders one verified citation group per concern, such as
`([#1754](https://github.com/OWNER/REPO/pull/1754))`. Sources associated with the
same PR produce one visible link in that concern. A subject does not need a PR
number. The model cannot supply citation URLs. Documentation links must come
verbatim from the source. Unknown IDs, duplicate IDs within a group, malformed
or misplaced attribution, and unverified links block publication and cache
writes, even with `force`. The coverage check runs for unprefixed tags and
fails publication when a selected source is missing. Invalid attribution
withholds preview text
rather than exposing internal tokens or unchecked links.

GitHub paginates commit associations and links all merged PRs in the requested repository, sorted by number. An empty association array permits a fetched commit link with a short SHA. Open-only associations and API, permission, or transport failures remain unresolved. GitHub Apps need **Repository permissions → Pull requests → Read-only** in their registration. Existing installations also need the installation owner's approval of that additional permission. See [GitHub's permission-change instructions](https://docs.github.com/en/apps/maintaining-github-apps/modifying-a-github-app-registration#changing-the-permissions-of-a-github-app).

Forgejo checks its singular commit-to-PR endpoint. For branch commits, it also checks verified PRs from merge SHAs collected in the same release range. Paginated PR commit membership establishes the exact association. Multiple verified range-merge associations are linked in number order.

Only Forgejo's explicit missing-record response permits a fetched commit fallback after these checks. Generic 404s and failed membership lookups remain errors. Rebase methods that leave no recorded association can produce commit links. A fallback reports that the platform recorded no association, not that no PR ever existed.

These rules apply in lite and deep modes through the shared webhook, API,
dashboard, and CLI pipeline. Rendering changes only the attribution group, with
no prose rewrite or extra model call. Successful publications cache final linked
Markdown and private attribution metadata: source SHAs for each citation span
and a hash of the exact note body. Cache hits recheck coverage, group uniqueness,
placement, and link targets without provider lookups. Corrupt or missing metadata
withholds the notes and blocks publication. The contract version invalidates old
entries even when an authoritative custom prompt is unchanged. Provider lookup
failures return HTTP 502 and record the safe history category `source_links`.

## 4. The three built-in voices

The built-in voices include editing rules adapted from
[STE80 Humanizer](https://github.com/Dodaios/ste80-humanizer). The editing mode is
part of each voice: `chronicler` uses Humanize, `engineer` uses Strict STE style,
and `launch` uses Light. The neutral default uses Humanize. There is no separate
setting or extra LLM call.

All modes remove AI-writing habits such as chatbot openings, inflated claims,
and repetitive closers. Humanize also simplifies sentence structure while
preserving the voice. Light keeps more of the original rhythm and changes only
the most complex sentences. Strict applies tighter sentence, vocabulary, and
verb rules. It aims for STE style, not certified ASD-STE100 compliance.

Editing changes wording only. Coverage, the required output shape, technical
names, numbers, and uncertainty take precedence over style. A recommendation
never becomes a requirement to satisfy Strict. Freeform custom tones do not
receive these editing blocks. Full-prompt overrides, including named profiles,
remain authoritative and receive none of the built-in voice instructions.

### chronicler

**Purpose:** Write for readers who want each release as a narrative chapter — the careful writer who records a project's history as it unfolds.

**Personality:** Warm, story-driven, plain-spoken. Frames the release as the next entry in an ongoing story. The short prose lead explains what drove the release and how it advances the project. Ordinary bullets use one short, factual sentence in that voice. Actionable changes retain the steps and conditions the reader needs. Avoids breathless hype or vague summaries.

**Fidelity:** Bound by the fidelity contract above. Narrative phrasing changes no coverage: every selected distinct change appears under its concern, with all source identities accounted for.

### engineer

**Purpose:** Write for maintainers, contributors, and technical readers who need precise, unembellished records.

**Personality:** Direct, terse, precise. Leads with a short statement of the headline changes. Uses exact technical terms and short factual sentences. Names a public interface or configuration when the source states it. No marketing language, cheerleading, or technically meaningless adjectives.

**Fidelity:** Bound by the fidelity contract. The technical voice changes nothing about change coverage — only that the phrasing is dry and specific.

### launch

**Purpose:** Write release notes for end users and stakeholders who care about what the change means for them.

**Personality:** Enthusiastic and product-forward. Opens with an upbeat prose paragraph about the most user-visible improvement, and lets each bullet lead with what the reader gains rather than what the code did. Keeps the energy strictly grounded in the log — no unsupported claims of size, speed, or impact — with crisp, active sentences that never go vague.

**Fidelity:** Bound by the fidelity contract. Benefit-first phrasing preserves every selected distinct change and required condition, independent of commit or PR boundaries.

## 5. Writing your own voice

Use this three-section template. Each section is one short paragraph of concrete instructions. Match the `Purpose / Personality / Fidelity` structure used by the built-in voices. A freeform custom `tone` is inserted as the persona but still receives the shared concise rules, whose coverage bullets restate the contract in section 3; only an authoritative full prompt (an in-repository instructions file or a named profile's prompt, section 6) bypasses the defaults, so a custom voice that wants a different shape must supply one of those instead.

```markdown
## Purpose
Write release notes for [audience, e.g. "platform operators and SRE teams"]. Focus on [what matters most, e.g. "operational impact and migration steps"].

## Personality
Tone is [2-3 adjectives, e.g. "authoritative, concise, and action-oriented"]. Prefer [specific phrasing style, e.g. "imperative mood and component-first descriptions"]. Avoid [what to skip, e.g. "marketing language and subjective claims"].

## Fidelity
[Paste the fidelity contract from section 3 above, or state: "Bound by the standard fidelity contract: one concern per primary bullet, every selected distinct change preserved, all contributing source identities attributed, tone changes wording only."]
```

**Small-model prompting tips:**

- Use short declarative sentences. Small models follow simple instructions better than compound ones.
- Write `MUST` and `MUST NOT` rules, not suggestions. "Must preserve the public name a commit states" is clearer than "try to keep names."
- Make coverage verifiable. Every selected source identity must appear in a valid attribution group. Review distinct concerns and required facts separately, since source counts alone do not prove semantic coverage.
- Replace vague length rules with a clear default: one short sentence for ordinary changes. Allow the detail needed for required actions and conditions.
- Place the most critical constraints first — models pay most attention to early instructions.

## 6. Named release-note profiles

The built-in voices and the in-repository instructions file resolve one effective system prompt per release. Named profiles extend that model: the repository owns a manifest of named prompt files, and each generation request selects exactly one profile. One repository can therefore serve several note streams — maintainer, customer-facing, compact — from the same source range.

### The manifest

Profiles are declared in `.annalist/release-notes.yaml` at the root of the consuming repository:

```yaml
version: 1
profiles:
  maintainer:
    prompt: .annalist/prompts/maintainer.md
  customer:
    prompt: .annalist/prompts/customer.md
  compact:
    prompt: .annalist/prompts/compact.md
```

The manifest is strict. `version` must be `1`. At least one profile is required, and **every** entry is validated — not just the one requested: profile names must match `^[a-z0-9][a-z0-9_-]{0,63}$` (lowercase, digits, `_`, and `-`; starts alphanumeric; at most 64 characters), and each `prompt` must be a non-empty, repository-root-relative slash path ending in `.md`. Absolute paths, empty or `.` segments, `..` traversal, unknown fields, and a second YAML document are all rejected. When a profile is selected, its prompt file must exist and be non-empty.

The selected prompt is the **entire** authoritative system prompt for that generation: no built-in persona, no legacy instructions file, and none of the shared concise-default rules from section 3 are layered on top — a named profile defines its own prose, style, and formatting, subject to the mandatory source-citation contract above. Write profile prompts as complete instruction sets (the tips in section 5 apply); [`examples/prompts/`](../examples/prompts/) has small, working examples.

### Selection

Profile selection is explicit, and **one request generates exactly one profile's output** — there is no batch endpoint and no output composition. There is no default profile: a request with no `profile` (or an empty one) never reads the manifest and follows the legacy path — the provider-specific in-repository instructions file (`.github/release-notes-instructions.md` for GitHub, `.forgejo/release-notes.md` for Forgejo) is still used, with the file read pinned to the source tag like every other generation-time read, and when the file is absent or unreadable Annalist falls back to the configured tone/persona as before. When a profile *is* named, only that profile's prompt is used and the legacy instructions file is intentionally ignored rather than composed. Webhook-driven generation never selects a profile; it always follows the legacy path.

Both the manifest and the selected prompt file are read **pinned to the source tag** — the `to_tag` of the request — so the notes are always generated from the prompt that existed at the version being released. The same authority applies on the legacy path: when the in-repository instructions file exists, it is used as the complete system prompt and bypasses the built-in persona and the shared concise defaults of section 3.

### `display_version`

A generation can present a version that differs from the source tag — for example, a beta tag `v2.0.0-beta.3` displayed as `2.0`. The optional `display_version` field must be trimmed, at most 128 UTF-8 bytes, and free of ASCII control bytes. It changes only the identity in the user message sent to the LLM; the system prompt is unaffected. Omitted, it resolves to the source tag and the user message is byte-identical to today's form, `Generate release notes for version <tag>. (changes since <from>)`. Present and different, the message names both identities: `Generate release notes for version <display>. (source tag <source>; changes since <from>)`. The `; changes since <from>` clause is omitted when there is no from tag.

### Failure behavior

Invalid request values fail before any platform work: an invalid profile name, or a `display_version` that is present but blank, is HTTP 400 from the API (a non-zero exit from the CLI). Missing or malformed configuration — a missing manifest, an unknown profile, an invalid manifest, or a missing or empty prompt file — fails the request with HTTP 422 and zero LLM calls; the CLI exits non-zero. Non-404 platform read failures and other generation failures propagate as HTTP 500, as today.

### Publication

A release body holds **one** selected profile's notes. Repeated `publish: true` calls for different profiles on the same release replace the same Annalist-owned body **last-write-wins**: the publishes serialize and the later one wins. Callers that need several outputs must generate each profile with `publish: false`, compose the final body externally, and publish once.

Idempotency follows the full generation contract. The note cache key covers the platform, owner, repository, release id, from/to tags, profile, resolved display version, and a digest of the effective configuration. The digest includes the system prompt, model, temperature, commit types, mode, max tokens, thinking level, LLM base URL, and an explicit source/generation/quality contract version. That version invalidates old cache entries even when a custom full prompt is unchanged. Eligible cached notes pass current output checks and source-completeness collection before reuse.

### Quality review and context limits

Deep mode includes each selected commit's own diff alongside the full selected commit messages, one `### <short sha> <subject>` section per commit. The fixed 256 KiB hunk budget is shared max-min fair across those commits: a commit whose diff fits keeps it whole, and the surplus goes to the larger changes, so a single large pull request cannot consume the evidence for every other change. A hunk that does not fit is clipped at a line boundary and marked, and a hunk with no room is omitted. Lockfiles, vendored trees, snapshots, minified assets, and test data appear only in the `--stat` summary. Any clipping or omission produces the reason `source_context_incomplete`. That reason is advisory: it tells the reader which evidence the model lacked, so the run keeps the warning but still publishes. A diff-collection error fails the request rather than presenting partial context as complete. Limited context does not excuse dropping facts already present in the commit bodies.

Every built-in voice checks primary prose paragraphs for more than two prose
sentences or 60 prose words and reports `primary_bullet_overloaded`.
Recognizable commands, URLs, identifiers, code, quotations, and nested required
actions do not count as generic prose. This is a bounded readability check, not
a semantic concern detector or a total note-length limit.

The built-in `engineer` voice also checks semicolons in editable prose.
Recognizable commands, URLs, technical spans, and quotations remain unchanged.
These prose checks do not apply to authoritative full prompts or custom tones.
The Engineer semicolon check still runs only when `tone` is `engineer` and there
are no full instructions. Source attribution and prose failures block
publication for every prompt. Source-context truncation never does.

Coverage is checked for unprefixed tags, where every supplied commit is in
scope. For a `<prefix>/vX.Y.Z` tag Annalist cannot derive the artifact's scope
from the tag, so it leaves out-of-scope sources to the model and does not
enforce total coverage.

A review-needed preview returns its captured notes, `quality_issues`, and `warning_code: "quality_review_required"`. Attribution failures withhold the note text and report `source_attribution_invalid`. A failed prose check or a failed source attribution triggers one corrective regeneration that names the reason; a second failure stops the run. The regenerated draft is validated from scratch, so a dropped token can recover and a forged link still fails. Transient provider responses (429, 408, and 5xx) retry up to three attempts with backoff and `Retry-After`; authentication and model errors do not retry. A publish request with blocking issues fails with HTTP 422 before any release edit or generated-note cache write. `force` cannot bypass the checks. The dashboard shows the reasons, and CLI previews print them to stderr and exit nonzero. Server history keeps its existing outcome/status values and records the general quality warning without notes or source text. A rejected publish is a failed run with error code `quality`.

The current reason codes are `primary_bullet_overloaded`, `strict_prose_semicolon`, `source_context_incomplete`, `source_attribution_invalid`, and `quality_check_required`. The first two block publication until a draft passes. `source_context_incomplete` is advisory and never blocks: the notes publish with that warning. `source_attribution_invalid` withholds the notes. `quality_check_required` rejects direct publication of notes without a current check bound to that exact output and generation contract. Successful remote publication followed by a local cache failure remains a published outcome with `cache_persistence`, not a quality rejection.

These checks are not a semantic coverage validator. Compare change identities, required actions, conditions, exceptions, and certainty against the source separately. Automated consumers must reject warned previews or nonempty `quality_issues` before publishing notes outside Annalist. The example workflows perform that check. Existing custom prompt files are not changed by these defaults or checks.
