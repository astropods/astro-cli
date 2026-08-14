# Two notification payload variables were always empty

## Summary

Two Novu workflows reference a payload variable that no call site ever filled, so the
template rendered a blank every time.

`security.key_changed` sent an empty `keyName` on every revocation. The revoke handler had
the token id and never read the name, so the email read `the ingestion key "" was revoked`.

`billing.dunning_suspended` sent an empty `account` on every suspension. The dunning sweep
works off account ids, which left a blank on the one message that most needs to name the
account.

Both were documented as "may be empty". Neither was ever populated, so the templates were
carrying a branch that ran on every send.

## Design

`Store.Revoke` returns the revoked key's name:

```go
func (s *Store) Revoke(accountID, id string) (string, error)
```

The name comes from `RETURNING name` on the existing `UPDATE`, so there is no second query.
That also lets `sql.ErrNoRows` come straight from `QueryRow` instead of the previous
`RowsAffected` count, which removes a branch rather than adding one.

The dunning worker resolves the account name through a `dunningAccountNamer` interface,
mirroring `accountNamer` in the observation evaluator. The lookup is best-effort: a failure
logs and sends the notification unnamed, because a suspension is worth reporting without
the name. The wiring site guards the assignment, since assigning a nil `*AccountStore` to an
interface field stores a non-nil interface holding a nil pointer, which would slip past the
worker's nil check and panic on the first lookup.

## Migration

No action required. No payload keys changed.

`keyName` now always arrives populated, because a key cannot be created without a name and
the revoke path returns before notifying if the lookup fails. A template branch guarding
that blank is no longer load-bearing, though it stays harmless.

`account` is best-effort. The lookup can fail, and the sweep sends the notification unnamed
rather than not at all, so keep an empty-tolerant branch for that one.
