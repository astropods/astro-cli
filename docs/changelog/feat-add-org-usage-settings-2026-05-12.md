# Org Usage Settings & Entitlement Fixes

## Summary

Organizations had no way to view their resource consumption or quota increase history from the settings UI — usage was only visible on personal accounts. Separately, two entitlement enforcement gaps allowed users to bypass plan limits: members of any role could submit quota increase requests, and accounts with no plan entry for a feature (e.g. knowledge stores) could still use it freely.

## Design

**Org usage settings**

The usage settings page is now available under org settings (`/settings/org/:orgSlug/usage`) in addition to personal settings (`/settings/usage`). The core display logic — stat cards, usage bars, quota request history — lives in a shared `UsageView` component that accepts an `account` prop. Both route-level pages are thin wrappers that resolve the correct account name and pass it down.

**Quota increase access control**

Quota increase requests (POST `/api/v1/accounts/:account/quota-increase`) now require `org:admin` permission, matching the existing requirement on the list endpoint (GET). Previously any account member could submit a request. On the frontend, the "Request increase" button is hidden for org members via a `canRequestIncrease` prop on `UsageView`; personal settings always passes `true` since it's your own account.

**Entitlement enforcement for missing plan features**

The entitlement middleware previously failed open when a feature was absent from a customer's entitlements response — treating "not in plan" the same as "unlimited access." This meant accounts on plans without knowledge store entitlements could still create knowledge stores.

The fix separates two distinct cases:
- **Feature absent from plan** — always blocked, regardless of the `enforce` flag. If OpenMeter doesn't return an entitlement for a feature, the account doesn't have it.
- **Quota exceeded** — blocked only when `enforce=true`. The `enforce` flag continues to act as a kill switch for overage enforcement during rollout.

API errors from OpenMeter continue to fail open so billing infrastructure downtime doesn't break user workflows.

## Migration

No action required. Org members will see the Usage tab in their org settings automatically. Accounts that were previously able to create knowledge stores without a plan entitlement will now receive a 402 response.
