# A self-maintaining internal docs system, and closing the coverage gap it exposed

## Summary

Internal docs (`docs/`) had no mechanism keeping them in sync with the code
they described. A doc could describe a design nobody shipped, or go silent
about a system that changed underneath it, and nothing would notice. In
practice this meant nobody could tell which doc to trust for a given area,
so the default was to read code from scratch every time, or trust whatever
was nearby, including code that itself predated a convention. Only 10 of
the codebase's real feature areas had any canonical doc at all.

This branch builds the mechanism, then uses it: an area map that says which
doc is canonical for which code path, a hook that nudges when the two
diverge, and a skill that pulls only the relevant docs into a task instead
of guessing which of 90+ files to open. It then runs that system across the
rest of the codebase, taking area-map coverage from 10 areas to 27, fixing
real doc-vs-code drift and a number of real (not just doc) bugs found along
the way, and extends the same "prefer the documented convention" principle
into actual CI enforcement instead of a rule nobody checks.

## Design

**The area map** (`docs/README.md`) is the index: `Area | Code paths |
Canonical doc(s) | Notes | Verify`. `Code paths` is a set of globs; `Verify`
is a shell command that actually exercises that area's code, not just
compiles it, checked by hand against real test output before being written
down (an early version had a markdown-table-escaping bug that let a
`-run` regex silently match zero tests while still exiting 0 — fixed, and
the table's own intro now warns against the specific failure mode).

**The hook** (`.claude/hooks/docs-map-check.mjs`) reads that same table at
the end of a Claude Code turn. If the turn touched a mapped code path
without touching its canonical doc, it nudges a check rather than blocking
— docs drift silently by default, so the fix is to make the check happen
automatically instead of depending on someone remembering. Hardened against
several real failure modes found by adversarial testing: a malformed table
row used to crash the parser instead of being skipped, a fuzzy path match
used to false-positive on an unrelated file with the same suffix, and the
script assumed `cwd` was the repo root, which broke it entirely when
invoked from a subdirectory.

**The `docs-map` skill** answers "which doc(s) should I read for this
area" from the map, so a task pulls in a tight, targeted set of docs
instead of an agent guessing. Subagents don't inherit it (or `agents.md`)
automatically — `agents.md`'s Documentation section now says explicitly to
name the relevant doc(s) in a delegating prompt, since that's the only way
the pointer reaches a subagent at all.

**The Development Workflow** (`agents.md`) makes the doc check part of a
standard six-step loop for any code change: find the relevant docs first
(now including the app's own `CLAUDE.md`, not just the area map), prefer
the existing convention over inventing a new one, resolve any deviation
instead of leaving it silent, verify the change for real, bring tests up
to it, write the changelog. A documented convention now explicitly wins
over code nearby that doesn't follow it — code that predates or ignores a
convention is inherited drift, not precedent, so the instruction is to
match the doc and fix or log the deviation rather than copy it forward.

**Coverage expansion.** The area map grew from 10 to 27 areas across
several passes: mapping already-documented-but-unlinked areas (GitHub
build integration, Notifications), the highest-churn undocumented area in
the codebase (blueprint registry and its deploy UI), a handful of
areas that were actively drifting fast with no doc anchor at all (cluster
configuration and placement, observation/alerts), and a long tail of
smaller but real gaps (variables/secrets vault, account lifecycle,
messaging/chat interfaces, AI Gateway, audit log, quota, experiment flags,
account profile). Each pass verified claims against running code and tests
rather than re-describing what a doc already said, and a follow-up
adversarial audit fact-checked a sample of specific claims in every new
doc against source, finding and fixing a further batch of wrong or stale
claims (a fabricated code-comment quote, an invented method name, a
transposed pair of validation limits, two docs stamped "verified today"
that still described a billing plan removed the day before).

**Convention enforcement.** The existing `local-theme/no-raw-theme-colors`
ESLint rule (raw Tailwind palette colors vs. semantic tokens) already used
a ratchet: stay at `warn`, but fail CI if the violation count grows past a
checked-in baseline, so existing debt doesn't have to be paid down at once
but can't grow either. This extends the same pattern to inline styles: a
new `local-theme/no-static-inline-style` rule flags a literal-valued
`style={{}}` property only for a small, deliberately narrow set of CSS
properties Tailwind already covers, and explicitly excludes any value that
calls a dynamic CSS function (`var()`, `color-mix()`, a gradient), since
those are computed or theme-driven values even though the AST sees a plain
string. `apps/astro-client/CLAUDE.md`'s styling rules are now annotated
with their real enforcement status and current violation counts, instead
of asserting a rule as settled fact when it's actually unenforced.

**Ratchet nudges, not just gates.** Direct engineering feedback: keep
linting a blocking PR gate, but keep the freshness check a non-blocking
weekly report, since drift is cumulative and not attributable to any one
PR. All three ratchet scripts now print a reminder, without failing, when a
PR's own change pushes the violation count below baseline, asking the
author to lower the baseline in the same PR rather than leaving the win as
unclaimed slack. Two stronger versions were rejected: CI auto-committing a
new baseline needs a bot identity and push permissions for a small win, and
hard-failing on any mismatch would fail an unrelated PR the moment some
other change shifted the repo-wide count, the same "innocent PR blocked by
someone else's change" failure mode ratchets exist to avoid.

**A doc can be wrong without being stale.** The freshness check only
catches drift relative to a doc's own Last-verified date; it can't catch a
claim that was never true, or one that rotted silently in an area nobody's
touched recently enough to trip it. Two mechanisms target that instead:
`agents.md`'s Development Workflow now asks for a doc's specific
load-bearing claim to be checked against current code before it's used to
override what's actually in front of you, and its writing-style rules ask
new doc content to say what would confirm a checkable claim, so a later
check doesn't have to re-derive it from scratch.
[`docs/04-guides/doc-honesty-audit.md`](../../04-guides/doc-honesty-audit.md)
adds a quarterly adversarial backstop for whatever those two miss, prompted
by a new scheduled Slack reminder; `doc-drift-log.md` entries now record
how each finding was actually caught, PR review, scheduled audit,
verify-on-use, or the freshness check, the only way to eventually tell
which channel is carrying its weight.

## Migration

New CI gates on relevant PRs, all ratchets (they fail only on growth, never
on pre-existing debt): a hook test suite runs when `.claude/hooks/**`
changes, a broken-doc-link/dangling-symlink check runs when `docs/**` or
its own checker changes, and `apps/astro-client` PRs now run an inline-
style budget check alongside the existing theme-color one. A pre-existing,
unrelated bug is also fixed as part of wiring the new rule's tests:
`apps/astro-client/vitest.config.ts`'s test `include` pattern never
covered `eslint-rules/*.test.js`, so the existing color-rule's own test
suite has never actually executed via `vitest run` or CI; it does now. A
new scheduled workflow (`doc-honesty-audit-reminder.yml`) posts to Slack
quarterly, reusing the existing `SLACK_BOT_TOKEN`/`SLACK_ASTRO_OPS_CHANNEL_ID`
secret and variable already configured for the stale-PR reminder; it never
blocks a merge.

No data migration. No behavior change for end users; this is docs,
tooling, and CI only.
