# Deployment history offered an old GitHub build as a new one

## Summary

The Deployment History panel showed "New build available" over a fresher
`ast push` deploy, pointing at a GitHub build from days earlier. The nudge is
the update affordance on the agent detail page, so the offered action was a
rollback wearing an upgrade's label.

## Design

**Inequality is not recency.** `githubUpgrade` picked the newest registered
build out of the GitHub status list and treated it as an upgrade whenever its
`build_id` differed from the deployed one. A deploy that came from `ast push`
has no `github_builds` row — that absence is exactly how the server derives
`source = "direct"` — so its build id matches nothing in that list and the
comparison is unconditionally true.

**A disabled query still reads its cache.** The status fetch was already gated
on `currentRecord.source === "github"`, but the readers were not, and
`['github', account, name]` is shared with the blueprint detail page. React
Query serves a cached entry to a disabled observer, so the stale builds arrived
anyway. The panel fills that entry itself: deploy from GitHub, then `ast push`,
and the source flips to `direct` while the data it cached stays behind. No
navigation required, which is why the bug came and went.

The gate now applies once where the data enters the component, so the fetch and
every read cannot drift apart. Direct deploys fall through to the server's
`latest_build_id`, which ranks by `published_at` and was already correct. The
build-in-progress card read the same cache ungated and had the same defect.

**The poll ignored its caller.** `useGitHubStatus` starts a second 5s query
gated only on the latest build being in flight. A cached pending build therefore
polled the endpoint for callers that had disabled the hook. It now repeats the
caller's gate.

Three `useMemo` wrappers around the derived nudges are gone. They guarded a
`find` over at most ten builds and cached nothing React Query's structural
sharing did not already hold stable; what they did cost was three dependency
arrays, and a missed entry there reproduces this bug exactly.

## Migration

None. Client-only, no API or schema change.
