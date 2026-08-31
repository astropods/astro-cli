# Tighten the comment rules, close a convention-following gap, and fix subagent delegation

## Summary

Three unrelated gaps in `agents.md`, found and closed the same day.

The "comments are rare and short, reasoning belongs in the changelog" rule
kept getting violated on real changes. It had no checkpoint that actually
catches a new comment before the PR ships, it said nothing about tests
(where the same over-explaining habit is worse, since the test name and
assertions already carry the information a comment would restate), and its
bar for adding a comment at all was generic enough that comment bloat kept
recurring, a pattern engineers have flagged as a real problem.

Separately, the Development Workflow's convention-following step only
covered matching a convention that already exists nearby or in a doc.
Absent one, the step gave no guidance at all, so the fallback in practice
was whatever satisfies the immediate ask, including a bespoke solution or
duplicated code with no local precedent to point at yet.

Separately again: a subagent spawned via the Agent tool starts with none
of this file's content, confirmed live across multiple subagent types.
The file already warned that subagents don't inherit it, but only told a
delegating prompt to inject docs-map awareness, not the Writing style or
Development Workflow rules that actually govern comments and tests. Any
coding work delegated this way ran on unprompted model defaults no matter
what the rest of this file said.

## Design

**A checklist step, not just a rule.** Step 6 of the Development Workflow
(writing the changelog) is already "look at the whole change one more time",
so it's the natural place to also scan for comments the change just added.
If one explains *why* a decision was made or what was rejected, that
reasoning moves to the changelog and the comment gets cut to a one-line
pointer or dropped. A rule with no matching step in the workflow only gets
followed when someone happens to remember it; this gives it the same
standing as "run the tests" already has.

**Tests get their own line in the Writing style rules.** A test's name and
assertions are its documentation: a well-named test already states the
behavior and condition being checked, and an assertion message already
states what was expected and why. A comment repeating that adds a second,
driftable copy of the same information. The rule keeps one exception, for
something neither a name nor an assertion can carry, such as a dependency
serializing a request differently than its field names suggest.

**Exclusion is the default, and the bar targets a coding agent's
interpretive need specifically.** The old bar ("would genuinely confuse a
reader") is reader-generic and easy to satisfy for almost any line, since
most code could theoretically confuse somebody. The new bar asks whether a
comment would meaningfully improve a coding agent's ability to interpret
the code correctly: an external constraint, a non-obvious invariant, a
library's undocumented behavior, not friendliness or narration. A comment
that clears the bar still can't carry reasoning: no why, no rejected
alternative, no history, even when a comment is otherwise warranted. That
distinction matters, because the old rule only forbade history in a
comment that was already just restating "the code as it is"; a comment that
did earn its place could still smuggle in a "why" the changelog should
carry instead.

**A new Development Workflow step for when no convention exists yet.**
Requires searching for an existing shared implementation before writing a
new one, extracting code on its first duplication rather than its second or
third, and building the well-known solution for the kind of problem over an
ad hoc one that merely works today, even when the well-known solution takes
longer to land. This sits between the existing "prefer the existing
convention" step and "resolve a deviation" step, both of which assume a
convention is already there to follow or deviate from.

**Delegation prompts now name the rules, not just the docs-map.** The
existing subagent-delegation paragraph told a delegating prompt to inject
docs-map awareness when a task touches a mapped path. Extended it to say
the same for the Writing style and Development Workflow rules on every
coding delegation, and to tell the subagent to read `agents.md` itself
rather than have the delegating prompt restate the rule text inline,
consistent with reference over duplication elsewhere in this file.

## Migration

Doc-only. No action required, beyond knowing that a subagent needs the
relevant rules named or pointed at in its own prompt; it does not pick
them up on its own.
