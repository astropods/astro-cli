# Usage & Quota Increase Access for Account Members

## Summary

The `/usage` and `POST /quota-increase` endpoints were restricted to account admins (`org:admin`), preventing regular members from viewing usage data or requesting quota increases. This change opens both endpoints to any account member.

## Design

A new `RequireAccountMember` middleware was added to `internal/middleware/account.go`. It calls `accountStore.IsMember` for both personal and organization accounts, providing a reusable membership check without requiring a specific permission claim — unlike `RequireAccountPermission`, which requires a named permission present in the WorkOS JWT.

A new `accountMember` route group in `main.go` uses `ResolveAccount` + `RequireAccountMember`, mirroring the existing `accountAdmin` group pattern. The `/usage` (GET) and `/quota-increase` (POST) routes were moved from `accountAdmin` into this group. The `GET /quota-increase` (list requests) remains admin-only.

The `GetAccountUsage` handler was reverted to its original shape — membership enforcement is now handled entirely by the middleware layer.

## Migration

No migration required. Existing admin users retain access; members now additionally have access to these two endpoints.
