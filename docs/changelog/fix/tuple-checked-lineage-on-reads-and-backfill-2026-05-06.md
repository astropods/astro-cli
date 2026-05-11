# Tuple-checked lineage on reads and backfill

## Summary

Deployment **publisher attribution** and **upgrade lookups** could disagree with `agent_versions`: a reclaimed `source.account` name or a stale `source_account_id` pointed at an account that never published the deployed `(agent_name, build_id)`. The API could show the wrong `source_account` or steer `latest_build_id` at the wrong lineage. This change applies the same existence check used on writes (PR3’s `LineageValidator` / `ValidateLineage`) when resolving publisher display names, when choosing the account for batch latest-build resolution, during interactive template prefill, and in the startup **source_account_id** backfill.

## Design

**Read path (`handlers/deploy.go`)**  
`validatedLineagePublisher` centralizes resolution: try `source_account_id`, then `deployment_spec_json.source.account`, then the owning `account_id`, and return a publisher only if `ValidateLineage(publisherID, agent_name, build_id)` succeeds (or validation is inactive: no validator, missing name/build, or a nil interface holding a nil `*agentindex.Index`). If the column or spec path fails but `agent_versions` still has the tuple under the target account, same-account attribution still works. If `GetByID` fails after a successful tuple check (e.g. soft-deleted account), the code keeps the UUID for upgrade joins and falls back to `source.account` text from the spec for the display name — preserving the previous behavior for that edge case.

**List `latest_build_id`**  
`populateLatestBuildIDs` takes a single `*agentindex.Index` (batch query plus tuple validation) and uses `validatedLineagePublisher` per deployment for lineage joins.

**Backfill (`deploymentstore/spec.go`)**  
`BackfillSourceAccountIDs` selects `agent_name` and `build_id`, and only applies a publisher `source_account_id` when the validator (when set) accepts the tuple; otherwise it falls back to the target account if that tuple validates, or skips the row and increments `SkippedInvalidLineage`. When `source.account` resolves to an existing account whose tuple fails validation, `SkippedInvalidLineage` increments before evaluating the owning-account fallback.

**Typed-nil safety**  
`EffectiveLineageValidator` normalizes validators so a `(*agentindex.Index)(nil)` passed as `LineageValidator` does not panic `ValidateLineage`.

**Integration tests**  
`TestBackfillSourceAccountIDs_SpecNamesPublisherButRowBuildNotOnPublisher` registers the row’s `(agent_name, build_id)` on the **target** account only: with tuple validation enabled, the scenario only makes sense if the owning account actually publishes that build after the spec-named publisher is rejected.

## Migration

No database migration. Operators may see **publisher names or upgrade hints disappear** for rows whose stored lineage never matched a published build; that is intentional. Re-publish or repair data if those hints should return.
