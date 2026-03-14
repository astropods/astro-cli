# OpenMeter Usage Metering, Billing Plans & Admin UI

## Summary

Completes the OpenMeter integration for usage-based metering and entitlement enforcement. Astro now tracks resource consumption per account, auto-subscribes new accounts to the Private Beta plan, and enforces entitlement limits on resource-consuming operations. The admin UI (astro-queen) gains full plan/subscription/customer management capabilities.

## Design

### Metering & Plans

Five meters track all billable dimensions: `compute` (CU-hours), `agents` (count), `agent_builds` (count), `agent_deployments` (count), and `members` (count). A single "Private Beta" plan provides free access with hard limits on all meters. On server startup, `ValidateMeters` checks that all required meters exist in OpenMeter and logs errors for any that are missing.

### Auto-Subscription

New accounts are automatically subscribed to a default plan controlled by `OPENMETER_DEFAULT_PLAN` (set to `private_beta` in both preview and prod). After creating the OpenMeter customer, `CreateSubscription` is called with the plan key. Failure is logged but non-blocking.

### Entitlement Enforcement

An `Entitlements` struct (created once at startup) provides two methods:
- `Wrap(handler, features...)` — wraps a Gin handler for middleware-style route guards
- `Check(ctx, accountID, features...)` — for inline checks where the account is resolved differently

Enforcement is controlled by `OPENMETER_ENFORCE=true` (preview only, disabled in prod). When disabled, exceeded entitlements are logged but not blocked. When enforced, blocked requests return 402 with usage/limit details.

Enforced routes:
- **RegisterAgent** → `agents`, `agent_builds`
- **AddMember** → `members`
- **DeployAgent** → `agent_deployments`, `compute` (inline check, account from deployment spec)

### Customer Management (OpenMeter API alignment)

- `Customer` type updated to use `primaryEmail` (matching the API), `timezone` removed (not in API)
- Customer updates send full replace body (`name`, `key`, `primaryEmail`, `currency`) as required by `CustomerReplaceUpdate` schema
- Currency set to `USD` on customer creation
- v2 entitlements/grants endpoints now correctly unwrap paginated `{ items: [...] }` responses

### Admin UI (astro-queen)

- **Plans page**: Pretty/JSON views stay synced (JSON→Pretty parses back to form state); `duration` always sent in plan creation (required by API); plan list shows entitlement type, limits, and price columns
- **Customers page**: Bulk actions with shift/cmd-click selection — set currency or assign subscriptions to multiple customers at once; subscription status column
- **Customer detail**: Inline-editable fields (email, currency) as simple key-value list; Create Entitlement form renders root-level oneOf+discriminator schemas; Add Grant is toggleable
- **Sidebar**: Shows environment name (prod/preview) below title via `/api/env` endpoint

### Usage Page (astro-client)

New account usage settings page querying OpenMeter for current-month usage across all meters with entitlement limit display.

## Migration

- Set `OPENMETER_DEFAULT_PLAN=private_beta` to enable auto-subscription (already configured)
- Set `OPENMETER_ENFORCE=true` to enable entitlement enforcement (preview only)
- Ensure the `private_beta` plan is created and published in OpenMeter before enabling
- Existing accounts without subscriptions can be bulk-subscribed via astro-queen's customer page
