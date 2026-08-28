# The resource backfill no longer creates Insights resources

## Summary

Insights stopped being an FGA resource type, but the WorkOS resource backfill still synthesized one Insights row per account and registered it as a child of the account. The two changes landed in the wrong order, so the Queen **Connect WorkOS** job would have written a resource whose type no longer exists in the model, and two packages' tests stopped compiling against the deleted `authz.InsightsResource` helper.

## Design

`authzbackfill`'s resource query is the inventory of what WorkOS should hold for an account. It was a `UNION ALL` over the tables that own each resource type, with one synthetic branch on top that emitted `('insights', accounts.id)` for every account. That branch was the only resource in the query with no owning table, which is the same signal that made Insights a bad resource type in the first place: nothing creates it, so there is nothing to enumerate.

Dropping the branch leaves the query enumerating only rows that exist: blueprints, deployments, variables, and knowledge stores. The account itself is still registered by the backfiller as the parent, not by this query. The backfill's resource list now matches the spec's backfill table in [`docs/01-spec/private-by-default-fgac-rollout.md`](../../01-spec/private-by-default-fgac-rollout.md), which already dropped Insights.

Verified against a scratch database loaded from `sql/astro-server/schema.sql`: an account with one blueprint and no other resources returns exactly one row, typed `blueprint`. See `go test ./internal/authzbackfill/... ./internal/authorizationadmin/...`.

## Migration

No product or user action.

No environment holds orphan `insights` resources: staging has no `insights` resource instances, and production has no FGA model configured yet. If any turn up, Queen's resource inventory reports them as `workos_only` and can delete them.
