# Fix quota request and acceptance flow

## Summary

Approving a quota increase request granted nothing. The admin approval only
flipped the request row's status and recorded the granted amount; it never wrote
the account's enforced limit, so the requesting account stayed blocked. The flow
also still treated metered consumption (compute, knowledge storage/compute) as
quotas, even though those are gated by billing, not by per-account limits.

## Design

Per-account resource limits are enforced from `account_limits`, resolved by the
quota checker. Requests are count-enforced resources only; metered consumption is
billed and gated by the binary billing-suspended check, not a per-account limit.

- **Approval applies the grant.** `ApproveQuotaIncrease` now runs one
  transaction: lock the pending request, mark it approved, and upsert
  `account_limits` for the request's resource with the granted amount as the new
  absolute limit. Approval and enforcement can no longer diverge.
- **Only count-enforced resources are requestable.** Request validation and the
  approval guard share a single source of truth (`quota.IsResource`), backed by
  the quota checker's resource set. Metered feature keys are rejected at request
  time and cannot be granted.
- **Metered features are no longer quotas in the UI.** Removed the compute
  quota-increase dialog and the per-account "compute limit" gating from the
  deploy forms, and dropped metered entries from the request/label maps. Compute
  is surfaced through billing; there are no individual compute limits.

Also removed a stray brace in the admin proto that left the source out of sync
with its generated code.

## Migration

None. No schema or API changes; `account_limits` already existed and is now
written on approval.
