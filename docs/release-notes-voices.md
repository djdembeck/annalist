# Release Notes — Voices and Fidelity

## 1. What makes a good changelog

A changelog records what changed in a release, one concern per entry. It is not a marketing newsletter, a commit log, or a patch diff — it is a structured, readable record of user-relevant changes.

**Core principles (sourced from [Keep a Changelog 2.0.0](https://keepachangelog.com/en/2.0.0/)):**

- Entries belong in one of six types: **Added**, **Changed**, **Deprecated**, **Removed**, **Fixed**, **Security**.
- Each entry addresses a single concern. Do not combine unrelated changes into one bullet.
- Group entries by type. Do not include empty type sections.
- Date or version-tag each release.
- The changelog replaces the raw git log as the canonical record of what changed.

**Additional fidelity guidance (sourced from [LiteLLM's RELEASE_NOTES_GENERATIONS_INSTRUCTIONS](https://github.com/BerriAI/litellm/blob/main/RELEASE_NOTES_GENERATION_INSTRUCTIONS)):**

- Categorize each change before writing it.
- Preserve the factual content of every commit; tone and phrasing are presentation layers, not filters on the facts.

## 2. Why LLM release notes lose fidelity

Large and small language models share three failure modes when generating changelogs:

1. **Omission.** The model drops commits it considers "unimportant." This is the most common error. A dependency bump or internal refactor that the model discards may have been the only commit in the release range. The model's importance filter is not the author's.

2. **Merging.** Two or more distinct commits collapse into one vague bullet. Example: if a release adds a diff-aware analysis mode and also hardens prompt parsing, a model may produce "quiet but important strengthening of our foundations" — one bullet that erases two features. This is the defining failure mode of the `chronicler` voice when run without explicit fidelity guardrails.

3. **Editorialization and invention.** The model adds adjectives, claims, or behaviors not supported by the commit log. "Significantly improved performance" appears when the commits say nothing about benchmarks. "Fixed a long-standing bug" appears when the commit says "fix null check."

The root cause is that models are trained to be helpful summaries, not faithful records. Without an explicit contract separating "what changed" from "how you phrase it," the model optimizes for readability over accuracy.

## 3. The fidelity contract

Copy-paste this block into any custom release-notes prompt. It constrains the model to a faithful, verifiable output.

```
## Fidelity rules

- Every commit in the log becomes exactly one bullet. No more, no fewer.
- Never merge two commits into one bullet, even if they touch the same file or area.
- Never omit a commit, even if it appears minor, internal, or routine.
- Never invent a change, behavior, or feature not described in the commit log.
- The persona or voice you adopt changes only the wording of each bullet, never the set of facts.
- Always preserve the specific nouns, component names, configuration keys, and behaviors named in each commit.
- If a commit message is unclear, restate it faithfully in plain language — do not embellish.
```

This contract works regardless of whether annalist provides only `<commit_log>` or later also `<diff>` context. It is diff-agnostic.

## 4. The three built-in voices

### chronicler

**Purpose:** Write for end-users and project stakeholders who want a readable narrative of the release.

**Personality:** Warm, story-driven, confident. Uses phrases like "we added" and "this means you can." Leads with the release's theme.

**Fidelity:** Bound by the fidelity contract above. The narrative voice applies to phrasing only — every commit still becomes one bullet, and no change is merged, omitted, or invented.

### engineer

**Purpose:** Write for maintainers, contributors, and technical readers who need precise, unembellished records.

**Personality:** Direct, terse, component-focused. Uses imperative or past-tense technical language. No preamble or theme paragraph.

**Fidelity:** Bound by the fidelity contract. The technical voice changes nothing about fact coverage — only that the phrasing is dry and specific.

### launch

**Purpose:** Write for public-facing release announcements, blog posts, or changelogs visible to non-technical audiences.

**Personality:** Enthusiastic, product-forward, concise. Highlights user value. Uses active voice and short sentences.

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
