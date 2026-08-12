# Set a writing style for comments and changelogs

## Summary

The repo had no stated style for code comments, changelogs, or specs. Public
product documentation has had one in `docs-public/AGENTS.md` for a while, but it
is scoped to the Fern site and says nothing about comments in Go or TypeScript.

The gap shows up most in agent-written code, where comments drift toward
narrating the change rather than describing the system: "this was broken
before", "once support enables this", references to a review thread. Those read
fine the day they land and are noise a month later, because the reader has none
of the context and no way to tell whether the note is still true.

## Design

Adopt the Google developer documentation style guide as the baseline, rather
than inventing one. On top of it: active voice and present tense, one idea per
sentence, one approved term per concept, second person and imperative for
instructions, comments that explain why the code exists instead of restating it,
at most one comment per function, no narration of the session that produced the
change, and no em dashes.

Two of those need a word. **One approved term per concept** is the rule most
often broken on purpose: writers vary wording to avoid repetition, and in
technical text a synonym reads as a second thing. **At most one comment per
function** puts a ceiling on prose density, because several comments in one body
usually means the code needs better names.

The em dash rule is the one with a real cost, because the existing codebase uses
them heavily and the rule makes those lines non-conforming. It earns its place
by overlapping with the sentence-length rule: an em dash usually joins two ideas
that read better apart, so removing one tends to fix the sentence too.

The section sits in the root `agents.md` because comments span every language in
the monorepo. `docs-public/AGENTS.md` stays authoritative for the public docs,
which have a different audience and their own word list.

## Migration

None. Existing comments are not being rewritten, so the rules apply to new and
changed lines only. Files will hold both styles until they turn over.
