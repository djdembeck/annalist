package engine

// The built-in voices below shape prose with the STE80 method: a first pass
// removes machine-writing habits, a second pass rebuilds sentences with the
// structure of simplified technical English. The rule text is adapted from
// ste80-humanizer (Copyright Dodaios, MIT) and humanizer by blader
// (Copyright Siqi Chen, MIT); see STE80-LICENSE.txt in this package.
//
// The blocks concatenate at compile time: each style constant is assembled
// from shared fragments, so a rule stated once is emitted once, and no
// prompt text is built at runtime.

// voicePreamble states the precedence that applies to every built-in voice:
// style changes phrasing, never facts, certainty, exact names, or the
// required note shape.
const voicePreamble = `## Voice style
The notes follow a simplified technical English (STE) voice style. The style rules below change how sentences are phrased, never what they say. They never change a fact, number, name, version, date, identifier, or qualifier of certainty from the log; they never change the exact **Breaking:** marker, the note shape, or the section structure. When a style rule would change any of these, the fidelity and format rules win.`

// aiTellsBlock is the shared first pass: remove machine-writing habits.
const aiTellsBlock = `## Removing AI tells
Before polishing, remove machine-writing habits from the prose:
- State the point directly. No "not just X, but Y" staging, no one-line closers that repeat the point, no sayings that sound deep, no staged run-ups ("Let's dive in"), no defending against objections nobody raised.
- No forced triads: keep three items only when the log has three. No em or en dashes in the prose; use a period, comma, colon, or parentheses. No stacked qualifiers; keep one qualifier when the log supports it.
- Plain words: swap overused AI vocabulary (delve, leverage, robust, seamless, crucial, pivotal, showcase, underscore, utilize, meticulous, landscape, tapestry) for the simple word. No inflated significance, no vague "associated with," no "highlighting" or "ensuring" riders, no sales language, no borrowed authority, and no invented sources.
- Use "is," "are," and "has" instead of "serves as," "stands as," and "boasts." No decorative bold labels, no decorative title-case headings, no chatbot residue, no "as of my last update" guesses, and no re-explaining what the reader already knows.`

// structureHumanize is the second pass at the softened level.
const structureHumanize = `## Sentence structure
Rebuild sentences with the structure of simplified technical English, softened:
- One idea per sentence. Two linked facts may share a sentence with "and" or "because." Keep sentences short; a few words over twenty is fine when a split would break the meaning.
- Active voice; name who acts. Simple tenses only: present, past, and "will."
- Put the action in the verb ("adds," not "makes the addition of"). No "-ing" chains. One plain word with one meaning; one term for one thing; no rotating synonyms. At most three nouns in a row.
- One topic per paragraph. No semicolons. In the final review, inspect prose punctuation and split any sentence joined by a semicolon into separate sentences with periods, preserving the relationship and all facts. Do not alter semicolons inside commands, URLs, identifiers, or quotations.
- "May" and "should" only for real uncertainty or advice present in the log, at the log's strength of certainty; "might fail" becomes "can fail," never "fails."`

// structureLight fixes only the worst sentences; the rest stay as written.
const structureLight = `## Sentence structure
From the STE structure rules, fix only sentences over about 30 words or carrying three ideas: split them, keep one idea per sentence, use active voice, and name the actor. Leave every other sentence as written.`

// structureStrict is the strict STE second pass.
const structureStrict = `## Sentence structure
Write the prose in strict STE style (never say the text "is STE-compliant"):
- One idea per sentence. Descriptive sentences have at most 25 words; procedural sentences and warnings have at most 20.
- Active voice; name the actor. Simple tenses only: present, past, "will," and imperative. No perfect or progressive tenses ("has opened," "is running") and no modal-passive forms ("can be adjusted").
- Keep articles, "that," and the verb. No contractions, no semicolons, no Latin abbreviations ("e.g.," "i.e.," "etc."); write "for example" and "and other."
- "Check" and "test" are nouns only: "do a check of the pipeline," not "check the pipeline."
- Modals are only "can," "must," and "will." Where the log carries "may" or "should," keep that uncertainty; express it in other words, for example "in some cases," and never upgrade it to "will."
- One plain word with one meaning; one term for one thing; at most three nouns in a row, split longer stacks with "of" or "for."
- One topic per paragraph, with six sentences at most. A warning comes before the step it protects.
- In the final review, compare each bullet with its commit subject and body. Keep every timing and qualifying condition, including before, after, during, if, only, and unless, with the bullet for that change. Do not move a condition into a generic introduction or another bullet. Split prose clauses joined by semicolons into complete short sentences with periods. Preserve facts and do not alter semicolons inside commands, URLs, identifiers, or quotations.`

// The three built-in voice styles: shared preamble and first pass, each with
// its own second-pass level.
const (
	styleHumanize = voicePreamble + "\n\n" + aiTellsBlock + "\n\n" + structureHumanize
	styleLight    = voicePreamble + "\n\n" + aiTellsBlock + "\n\n" + structureLight
	styleStrict   = voicePreamble + "\n\n" + aiTellsBlock + "\n\n" + structureStrict
)
