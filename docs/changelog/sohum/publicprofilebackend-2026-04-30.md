# feat(profile): server-side profile fields, orgs endpoint, and hearts list

## Summary

Implements the full backend surface for the public profile page. Accounts gain an extended `account_profile` table with bio, location, contact, and social fields; `pronouns` and `website` are first-class columns (not buried in the social_links array). Two new public endpoints expose org memberships and hearted blueprints; the PATCH endpoint gains input validation and test coverage.

## Design

**Schema** — A separate `account_profile` table (joined to `accounts` via `account_id`) stores: `bio` (varchar 500), `location` (varchar 100), `email` (varchar 255), `local_timezone` (varchar 50), `pronouns` (varchar 50), `website` (varchar 255), `social_links` (text[]). An `account_number serial` column provides a stable rank for the early-adopter badge (first 1000 accounts by `created_at`). All profile columns default to NULL; `website` and `pronouns` are standalone columns rather than entries in `social_links`.

**Extended endpoints**
- `GET /api/v1/accounts/:account` — `AccountResponse` now includes all profile fields and `account_number` (omitted when null).
- `PATCH /api/v1/accounts/:account` — accepts `{ bio?, location?, email?, local_timezone?, pronouns?, website?, social_links? }`. Validates max lengths per field. `website` must be a valid `http`/`https` URL.

**New endpoints**
- `GET /api/v1/accounts/:account/orgs` (public) — resolves the account's owner via `account_members`, then returns all organization accounts that owner belongs to.
- `GET /api/v1/accounts/:account/hearts` (public) — resolves the owner the same way, then returns cursor-paginated hearted blueprints (default 20, max 100). Returns `{ items, next_cursor? }`.

**Hearts list** — joins `agent_hearts → agents → accounts`, ordered by `hearted_at DESC`. Cursor is an RFC3339 `hearted_at` timestamp.

**account_profile seeding** — `Create()` and `CreateWithoutOwner()` now INSERT an `account_profile` row at account creation time (`ON CONFLICT DO NOTHING`). Previously the row was created lazily on first `PATCH /api/v1/accounts/:account`, which meant the `account_number` serial sequence never fired for users who never edited their profile. Eager seeding ensures every new account receives an `account_number` at registration.

**Test coverage** — Handler and store tests cover all new paths; all account mock rows updated to match the 17-column scan signature.

## Migration

- Atlas applies the schema changes automatically on deploy.
- `account_number` is `serial` — existing rows in `account_profile` already have a number. New accounts will receive numbers at creation. A one-time `UPDATE` to reassign in `created_at` order is still recommended so the early-adopter badge (≤ 1000) reflects true join order.
- No action required for new endpoints; they are additive.
