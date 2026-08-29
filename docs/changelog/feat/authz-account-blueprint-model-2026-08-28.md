# Teach Astro the account and blueprint permission vocabulary

## Summary

WorkOS is configured with 46 permissions and 19 roles across five resource
types, but Astro's code knew only the five deployment permissions. Everything
past deployments, from account-scoped checks to blueprint discovery, has to name
those slugs, and a slug Astro names that WorkOS does not have fails closed. This
change puts the vocabulary in code and pins it to the file the WorkOS model is
applied from, so the two cannot drift silently.

Nothing checks the new permissions yet. This is the model the next steps need,
not the enforcement itself.

## Design

**Actions.** Every permission in `scripts/workos-fga/model.json` has a constant.
Account permissions keep their subject's namespace (`member:manage`, not
`account:manage_members`), which is the naming the model already uses. Audience
and knowledge store have constants without Astro roles, because `account-admin`
holds their permissions and the catalog should state that bundle in full rather
than in part.

**Role bundles.** The account and blueprint role ladders carry their permission
lists now, rather than the slug alone. Each rung is written as the rung below it
plus what it adds, which is exactly how the ladder is specified, so a reader
sees the delta instead of comparing two long lists. `account-admin` is the
exception worth reading: it holds every child permission, because that is how
WorkOS propagates access down the resource tree without a per-resource
assignment.

**A contract test rather than trust.** `internal/authz/model_contract_test.go`
loads `model.json` and asserts three things for the types Astro registers roles
for: every role's permission set matches exactly, every WorkOS role has a
catalog entry, and the two permission vocabularies are the same set. Hand-typed
constants that mirror a vendor config are exactly the kind of thing that rots,
and the failure mode is a denial in production rather than a compile error.

**One resolver for every type.** `DeploymentAccountResolver` became
`ResourceAccountResolver`. It answers the same three questions (owning account,
WorkOS organization, and whether the resource is in the rollout at all) for
accounts, blueprints, and deployments, differing only in the query that reaches
the owning account. A type with no query returns an error rather than falling
through, so an unmodelled resource can never resolve to some other account's
row, and the per-request cache is shared across types.

## Migration

None. No schema change, no configuration change, and no behavior change: the new
permissions have no call sites and the resolver answers the same way it did for
the one type it used to serve.
