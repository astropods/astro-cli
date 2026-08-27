# Doc honesty audit

A recurring adversarial pass that asks a different question than the
freshness check does. `check-doc-freshness.mjs` asks "has the code moved
since this doc's Last-verified date." This asks "is this doc actually
true," regardless of whether anything nearby has changed recently — a claim
can be wrong the day it's written, or rot silently in an area nobody's
touched, and the freshness check has no way to catch either.

## Cadence

Quarterly. A Slack reminder posts on that schedule (see
[`.github/workflows/doc-honesty-audit-reminder.yml`](../../.github/workflows/doc-honesty-audit-reminder.yml));
it doesn't run the audit itself, since this needs judgment a script doesn't
have. Adjust the cadence once [`../07-feedback/doc-drift-log.md`](../07-feedback/doc-drift-log.md)'s
`Caught by:` tags show how much this channel is actually finding relative to
PR review and verify-on-use checks — tighten if it keeps finding real
issues, loosen if a quarter goes by clean.

## Running it

1. Pick a slice of `docs/` to cover this round — either everything under
   `01-spec` and `03-architecture` if it's been a while, or the areas
   `docs/README.md`'s area map hasn't had a dedicated audit pass on yet.
   Don't try to re-verify every doc in the repo in one pass; a smaller slice
   done adversarially beats a large one skimmed.
2. Dispatch one or more subagents against that slice with an explicitly
   adversarial framing: find specific, checkable claims (counts, function
   or constant names, quoted code, described behavior) and try to disprove
   each one against current code, not just check that the doc reads
   plausibly or is internally consistent. A doc that sounds confident and
   well-organized is exactly the failure mode this is hunting for — fluency
   isn't evidence.
3. For each subagent's findings, independently re-verify before acting on
   them; don't take an audit agent's claim at face value any more than the
   original doc's. This isn't paranoia for its own sake: on the first full
   audit that used this pattern, a subagent's own claim that git history was
   unverifiable turned out to be an environment issue on its end, not a
   real limitation.
4. Fix what's fixable on the spot. Log anything needing domain knowledge or
   a larger change in `doc-drift-log.md`, tagged `Caught by: scheduled
   audit`.
5. Run the usual verification suite (`check-doc-links.mjs`,
   `check-doc-stamps.mjs`, `check-doc-freshness.mjs`) once fixes land, same
   as any other doc change.

## What this doesn't replace

Verify-on-use (checking a doc's specific load-bearing claim against code
right before relying on it, per `agents.md`'s Development Workflow) is the
cheaper, higher-frequency check and should be catching most wrongness
before it reaches a scheduled audit. This exists as the backstop for docs
nobody's currently using or citing, where verify-on-use never triggers.
