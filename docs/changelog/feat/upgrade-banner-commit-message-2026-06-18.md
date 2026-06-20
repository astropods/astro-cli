# Upgrade banner shows the target build's commit and SHA

## Summary

The "New build available" nudge on the agent detail page previously identified
the upgrade target only by a truncated build-id transition (`a1b2c3d4 → e5f6…`),
which says nothing about what the new build actually contains. When a build came
from GitHub, the commit that produced it was already known to the system but
never surfaced for the *latest published* build. This change carries that
commit's message, SHA, and originating repo through to the banner so it reads
like the deployment tiles already do — by what changed, not by an opaque hash —
and links straight to the commit on GitHub.

## Design

The deployment-history tiles already display a build's commit message because
their query left-joins `github_builds` on `(account_id, agent_name, build_id)`.
The upgrade banner, however, sources its target build from the *blueprint's
latest version* (`agent_versions`), which carried no commit metadata.

Rather than materialize commit columns onto `agent_versions`, the account
blueprint list query reuses the existing join pattern: a `LEFT JOIN LATERAL`
against `github_builds` (joined to `github_connections` for the repo) resolves
the commit message, SHA, and `repo_full_name` for the latest version's
`build_id`. `build_id` is not unique in `github_builds` (retries reuse it), so
the lateral selects the most recent row (`ORDER BY enqueued_at DESC LIMIT 1`) to
keep the list at one row per agent rather than fanning out. Direct CLI pushes
have no matching build and yield empty fields.

The metadata flows through `AgentVersion` → `AgentVersionResponse`
(`commit_message` / `commit_sha` / `repo_full_name`, all omitempty) and into the
client `BlueprintVersion` type. The banner renders the first line of the commit
message, with a second line carrying the GitHub mark and the short SHA linked to
the commit (`/commit/<sha>`, deterministic from repo + SHA). Builds with no
commit (direct CLI pushes) fall back to the original build-id transition with no
second line.

Only the account-list path is joined; the blueprint-detail and public-list paths
are unchanged (they leave the fields empty, which the client tolerates).

## Migration

None. No schema change; the new response fields are additive and optional.
