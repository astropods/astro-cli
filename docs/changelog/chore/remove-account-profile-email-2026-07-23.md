# Remove the account_profile email field

## Summary

`account_profile.email` was an optional, user-editable public-profile contact email — written only via profile settings, unset at account creation, and easily confused with a user's real identity email. It was unreliable as a billing/identity source and duplicated the WorkOS-verified email we already mirror. This removes the field end to end.

## Design

The canonical identity email is the account owner's WorkOS email, mirrored in `account_member_emails` and read via `AccountStore.GetOwnerEmail`. Billing/payment customer provisioning already uses that; nothing of value depended on the profile email.

Removed:
- **Schema** — the `email` column on `account_profile`.
- **Backend** — `Account.Email`, the column from every account SELECT / `scanAccount`, the `UpdateProfile` parameter, and the `PATCH /me` request field + validation. The Metronome lazy-provisioning path now sources the owner email via `GetOwnerEmail` (matching the Stripe path).
- **Frontend** — the email input on the profile edit sidebar and the mailto row on the profile view; the field is dropped from the `AccountPublic`/`Account` types and the profile-update mutation.

The WorkOS user identity email (`User.email`, the `/me` `user.email`) is unaffected — only the editable profile field is gone.

## Migration

Additive-safe schema change applied by Atlas from `sql/astro-server/schema.sql` (column drop; any stored profile emails are discarded). No API consumer action beyond dropping the now-removed `email` field from profile reads/writes; sending it is ignored.
