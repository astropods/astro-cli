# Serve an account's allowed clusters

## Summary

`account_clusters` records which clusters an account may deploy to, and only the admin RPCs read it. A deploy form that wants to offer a region has nowhere to ask, and the deployment-template endpoint is the wrong place: its job is to resolve one choice, not to enumerate them.

## Design

**The account read carries the list.** `GET /accounts/:account` gains `allowed_clusters`: the cluster id, its region, a display label and flag for that region, and which binding is the default. It joins `account_clusters`, so a caller that already loads the account for a page gets the list without a second request. The field is omitted when no cluster is registered at all.

It inherits the endpoint's existing public visibility. A cluster's region is no more sensitive than the display name and location that endpoint already serves.

**Reading an account binds the primary.** Most accounts have no row in `account_clusters`, and an empty list is ambiguous: it can mean "unrestricted" or "restricted to nothing", and every consumer would have to know that the primary cluster is allowed even when absent. The read writes the primary in as a real binding the first time it finds none, so the table is the whole answer and the read path is pure afterwards. It is one write per account.

Nothing to bind stays a valid state. With no `clusters` row for the primary, which is the local mode with no cluster config, the insert matches nothing and the account stays unbound.

**A failed binding read is not fatal.** The account payload renders without `allowed_clusters` and the failure is logged. A profile page should not return 500 because one join failed, and a caller that finds no list offers no choice.

## Migration

None. `allowed_clusters` is additive, and clients that ignore it are unaffected.
