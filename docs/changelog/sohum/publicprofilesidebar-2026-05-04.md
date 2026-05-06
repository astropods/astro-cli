## Summary

Wires the public profile page sidebar to real API data. Replaces all mock constants (`LOCAL_PROFILE_OVERRIDES`, `LOCAL_ORG_OVERRIDES`, `MOCK_HEARTED`) with live queries backed by the endpoints introduced in PR 1. Extends the `Account` and `AccountPublic` types with profile fields so data flows naturally from the session and public account responses.

## Design

**New query hooks** (`src/api/queries/accounts.ts`):
- `useAccountOrgs(account)` — fetches `GET /api/v1/accounts/:account/orgs` for the org logo strip in the sidebar
- `useListHearted(account)` — fetches `GET /api/v1/accounts/:account/hearts` for the Hearts tab

**API layer** (`src/lib/api.ts`):
- `AccountPublic` extended with `account_number`, `linkedin_url`
- `Account` (session type) extended with all profile fields so `ProfileSettings` can read from `personalAccount` directly
- New interfaces: `AccountOrg`, `AccountOrgsResponse`, `HeartedItem`, `HeartedListResponse`
- New methods: `getAccountOrgs`, `listHearted`

**Query keys** (`src/api/queries/keys.ts`):
- `accountKeys.orgs(account)` and `accountKeys.hearted(account)` added

**AccountProfile page**:
- Social links array now includes all five fields (astro domain, website, X, LinkedIn, GitHub) with real `href` targets and correct icons
- Early adopter badge is conditional: only shown when `account_number != null && account_number <= 1000`
- Org section driven by `useAccountOrgs()` for non-member visitors; members still use their session `accounts`
- Hearts tab driven by `useListHearted()` — empty state distinguishes "no hearts yet" from "no matches"
- `BlueprintCard.description` made optional so hearted items (which have no description from the server) render cleanly

**Settings** (Taylor's `ProfileSettings` page included via cherry-pick):
- Profile tab covers display name, bio, website, X, LinkedIn, GitHub in one save
- Uses `useUpdateAccountProfile` mutation (invalidates account detail query on success)

## Migration

No user action required.
