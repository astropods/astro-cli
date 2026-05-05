## Summary

Fixes four bugs in the public profile backend: a destructive PATCH upsert that wiped unmentioned fields, unauthenticated access to org membership lists, missing URL/email format validation, and confirms account_number assignment is already handled at registration.

## Design

**Destructive upsert (Bug 1)** — `UpdateAccountRequest` profile fields (`bio`, `location`, `email`, `local_timezone`, `pronouns`, `website`, `social_links`) are now pointer types. A `nil` pointer means "field absent from request, leave unchanged"; a non-nil pointer (including to an empty string) means "set to this value". The SQL upsert uses `CASE WHEN $N THEN EXCLUDED.col ELSE account_profile.col END` for each field, with boolean "was-provided" parameters $9–$15. If no profile fields are provided at all, the profile upsert is skipped entirely (only `display_name` update runs).

**Unauthenticated org exposure (Bug 2)** — `GET /api/v1/accounts/:account/orgs` now runs behind `OptionalAuth` middleware (a new sub-group in `setupRoutes`). The handler checks `GetUser(c)` against the account's first member ID; if the caller is unauthenticated or is a different user, it returns an empty list without hitting the orgs query.

**Missing validation (Bug 3)** — Added `net/mail.ParseAddress` check for non-empty `email` values and `url.ParseRequestURI` + scheme check (`http`/`https`) for `website` and each entry in `social_links`.

**account_number placement (Bug 4)** — No change needed. `store.go` already seeds the `account_profile` row (and thus assigns `account_number`) at account creation time via `INSERT … ON CONFLICT DO NOTHING`.

## Migration

No migration required.
