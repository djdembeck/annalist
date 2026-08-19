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
- Preserve the factual content of every commit; tone and phrasing are presentation layers, not filters on the facts.

## 2. Why LLM release notes lose fidelity

Large and small language models share three failure modes when generating changelogs:

1. **Omission.** The model drops commits it considers "unimportant." This is the most common error. A dependency bump or internal refactor that the model discards may have been the only commit in the release range. The model's importance filter is not the author's.

2. **Merging.** Two or more distinct commits collapse into one vague bullet. Example: if a release adds a diff-aware analysis mode and also hardens prompt parsing, a model may produce "quiet but important strengthening of our foundations" — one bullet that erases two features. This is the defining failure mode of the `chronicler` voice when run without explicit fidelity guardrails.

3. **Editorialization and invention.** The model adds adjectives, claims, or behaviors not supported by the commit log. "Significantly improved performance" appears when the commits say nothing about benchmarks. "Fixed a long-standing bug" appears when the commit says "fix null check."

The root cause is that models are trained to be helpful summaries, not faithful records. Without an explicit contract separating "what changed" from "how you phrase it," the model optimizes for readability over accuracy.

## 3. The fidelity contract

Copy-paste this block into any custom release-notes prompt. It is byte-for-byte
identical to the `fidelityBlock` constant in `internal/engine/personas.go`, which
every built-in persona emits verbatim; the shared rules block restates the same
rules. It constrains
the model to a faithful, verifiable output.

```
## Fidelity
Report every commit in the log exactly once. Write one bullet per commit. Never merge two distinct changes into one bullet. Never omit a commit. Never invent a change, component, or behavior not present in the log. Keep the specific nouns and behaviors of each change intact. Your tone may alter phrasing, but never which facts appear or how many changes are reported.
```

**Optional addendum** — not part of the shipped `fidelityBlock`. If your commit
messages are terse, append this line below the block to keep phrasing honest
without expanding the fact set:

```
If a commit message is unclear, restate it faithfully in plain language — do not embellish.
```

This contract is diff-agnostic: it applies whether the prompt carries only `<commit_log>` or also a `<diff>` block.

## 4. The three built-in voices

### chronicler

**Purpose:** Write for readers who want each release as a narrative chapter — the careful writer who records a project's history as it unfolds.

**Personality:** Warm, story-driven, plain-spoken. Frames the release as the next entry in an ongoing story; opens with a prose paragraph that sets the scene — what drove the release and how it advances the project; each bullet reads like a journal entry, not a log line. Avoids breathless hype or vague summaries.

**Fidelity:** Bound by the fidelity contract above. The narrative voice applies to phrasing only — every commit still becomes one bullet, and no change is merged, omitted, or invented.

### engineer

**Purpose:** Write for maintainers, contributors, and technical readers who need precise, unembellished records.

**Personality:** Direct, terse, component-focused. Leads with a short, direct prose statement of what changed and any caveats. Names components, modules, and behaviors; uses exact technical terms; keeps sentences short and factual. No marketing language, no cheerleading.

**Fidelity:** Bound by the fidelity contract. The technical voice changes nothing about fact coverage — only that the phrasing is dry and specific.

### launch

**Purpose:** Write release notes for end users and stakeholders who care about what the change means for them.

**Personality:** Enthusiastic, product-forward, concise. Leads every sentence with the reader's benefit; opens with an energetic prose paragraph highlighting the most user-visible improvement; each bullet starts with what the reader gains, not what the code did. Uses crisp, active voice and short sentences — excited but grounded in specifics, never vague.

**Fidelity:** Bound by the fidelity contract. Enthusiasm applies to tone, not to content. No commit is skipped, merged, or invented.

## 5. Writing your own voice

Use this three-section template. Each section is one short paragraph of concrete instructions. Match the `Purpose / Personality / Fidelity` structure used by the built-in voices.

```markdown
## Purpose
Write release notes for [audience, e.g. "platform operators and SRE teams"]. Focus on [what matters most, e.g. "operational impact and migration steps"].

## Personality
Tone is [2-3 adjectives, e.g. "authoritative, concise, and action-oriented"]. Prefer [specific phrasing style, e.g. "imperative mood and component-first descriptions"]. Avoid [what to skip, e.g. "marketing language and subjective claims"].

## Fidelity
[Paste the fidelity contract from section 3 above, or state: "Bound by the standard fidelity contract: one bullet per commit, never merge/omit/invent, tone changes wording only."]
```

**Small-model prompting tips:**

- Use short declarative sentences. Small models follow simple instructions better than compound ones.
- Write `MUST` and `MUST NOT` rules, not suggestions. "Must preserve component names" is clearer than "try to keep names."
- Make rules verifiable. "Every commit becomes one bullet" can be checked. "Write clearly" cannot.
- Avoid vague adjectives doing the work. Instead of "write concisely," say "one sentence per bullet."
- Place the most critical constraints first — models pay most attention to early instructions.
