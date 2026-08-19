# Require an Astro identity before mirroring org memberships

## Summary

Members in the organization member list could render as a raw WorkOS user ID. The name, handle, and avatar on a member row all come from that user's personal account, and the local membership row is written before the personal account exists, so any user who accepted an invitation and abandoned onboarding produced a row with nothing to display.

The ordering was the defect, not the missing label. Login calls `SyncMembershipsForUser`, which mirrors WorkOS memberships into `account_members`. Onboarding creates the personal account, and a user reaches onboarding only after their first login. Every abandoned onboarding therefore left a member row that no consumer of the member listing could render: the org settings table fell through to the user ID, and the grants member picker rendered an empty label and a bare `@`. Pending invitations rendered the same way, for a different reason: an invitee has no personal account by definition.

## Design

Mirror a membership only for users who have an Astro identity, and complete the mirror the moment that identity appears.

`SyncMembershipsForUser` now checks `HasPersonalAccount` once, before any upsert, and returns early when the account is missing. The skip is the expected state for a first login, not an error. `CreateAccount` then calls the same sync after it creates a personal account, so memberships held in WorkOS since the invitation was accepted land in the same request that finishes onboarding. That call is best-effort: the next login retries. `Sync.AddMember` carries the same check, closing the one other path that can write a membership for an arbitrary user ID.

The invariant becomes: a row in `account_members` has a personal account behind it, so a profile-less row is now the exception rather than a state every reader has to expect. The member listing and the grants picker still fall back through the same chain, since rows written before the requirement keep their membership.

One consequence is intentional. A user who has accepted an invitation but not finished onboarding now holds no local membership, so org-scoped requests are rejected until they do. The web client already routes them to onboarding before anything else, and the CLI requires an account.

Pending invitees can never satisfy that invariant: they have not accepted yet, so no personal account exists to name them. They are named by the address they were invited at instead. `ListMembers` makes one pass over the assembled rows, local and pending alike, and names every row that has no profile. That covers invitees and the memberships mirrored before the identity requirement, which are the same shape from the reader's side. It reads the local email mirror (`account_member_emails`) first, asks WorkOS for the ones it misses, and writes each answer back to the mirror so later loads stay local. The mirror's own backfill job only walks `account_members`, so it never reaches an invitee; this read is what seeds them. Lookups are capped per request, run eight at a time, and share a two-second budget, so what they can add to a listing is bounded no matter how many members are unnamed or how slow WorkOS is. A resolved address is permanent, so a large invite batch names itself over the next few loads rather than in one burst. A member WorkOS cannot name stamps `member_email_reconcile_attempts` and waits out the same `RetryBackoff` the reconcile job honors, so an unnameable row costs one lookup per window instead of one per view. Both writes run on a context detached from the request: a heal dropped halfway would buy the same lookup again on the next listing. Every step is best-effort: an unreachable WorkOS leaves the placeholder in place rather than failing the listing.

The email is returned only for pending rows with no profile, and only on a listing the caller is already a member of.

One state still resists naming. Deleting a personal account leaves the user's org memberships in place, and the identity check counts soft-deleted accounts, so the mirror keeps the membership while the profile lookup returns nothing. Their email usually still resolves, so the row is named anyway; only a user with no recorded email falls all the way through. Aligning the two queries would also let a user with a deleted personal account create a second one, which is a separate decision.

Whatever fails to resolve renders under a placeholder, so the UI keeps a floor: `"Unknown user"` in muted italics with the user ID in the tooltip for support lookups, matching what `UserBadge` already does on trace and spend surfaces. An unnamed row also drops the ID-derived avatar URL, which was a guaranteed 404 against the avatar CDN, in favor of the placeholder avatar. The row stays fully manageable so an admin can still change a role or remove the member.

## Migration

None. No schema or data change. Existing unnamed rows read as "Unknown user" instead of an ID, and they resolve to a real name on their own once the person finishes onboarding.
