# Billing provider seam + quota split (Metronome migration, Phase 1)

## Summary

First step of replacing OpenMeter with Metronome behind a provider interface (see
`docs/01-spec/metronome-billing-implementation.md`). Two independent changes:
(1a) a provider-agnostic billing seam that routes all metering and
customer-lifecycle paths through an interface, and (1b) a DB-backed per-account
quota checker that takes over resource-count limits from the OpenMeter
entitlement path. Behavior is unchanged: OpenMeter remains the only metering
backend (reached through an adapter), and quota enforcement respects the same
enforce flag as before.

## Design

**Interface.** New `internal/billing` package defines `BillingProvider`
(customer lifecycle, `IngestUsage`, `CheckBalance`, `GetUsage`) and the
hosted-only `HostedBilling` (`GrantCredits`, `ProvisionPackaging`), plus shared
provider-agnostic types (`UsageEvent`, `Balance`, `Account`, `UsageReport`,
`PackagingPlan`, `ErrUnsupported`). Metering code depends only on this interface.

**Package move + adapter.** `internal/openmeter` moved to
`internal/billing/openmeter`. A new `openmeter.Provider` adapts `*openmeter.Client`
to `BillingProvider`/`HostedBilling`. The adapter maps the provider-agnostic
`UsageEvent` to the OpenMeter CloudEvent wire format, carrying the event UUID
through as the CloudEvent `id` so idempotency and backfill dedupe are unchanged.
`NewProvider(nil)` returns a true nil interface so the existing
"no client ⇒ no-op" guards collapse into a single `provider == nil` check.

**Metering source through the interface.** The emit helpers,
`BillingStateManager`, and `Heartbeat` now build `[]billing.UsageEvent` and call
`provider.IngestUsage`, instead of building CloudEvents and calling
`client.IngestEvents`. The CU-hour math, the two billing-state tables, and the
heartbeat/reconcile state machine are unchanged — only the sink moved.

**Wiring.** `Clients.Billing billing.BillingProvider` and
`riverqueue.Config.Billing` replace the concrete `*openmeter.Client` on the
metering/customer paths (accounts, agents, org, deploy, knowledge handlers; the
heartbeat, backfill, github-build, and purge workers). The concrete client is
retained transitionally as `Clients.OpenMeter` for the two read endpoints
(`GetAccountUsage`, `GetInfrastructureUsage`) that use `GetCustomerAccess` /
`QueryMeter`, and for the entitlement middleware — both move in later phases.

## Design — quota split (1b)

**Separation.** The former OpenMeter entitlement check conflated two concerns:
per-account resource *counts* (max agents, builds/period, deployments, members,
knowledge stores/endpoints) and metered *consumption* (compute, knowledge
storage). These now split cleanly:

- **Quota** (`internal/quota`) — DB-backed resource-count limits, enforced for
  OSS and hosted alike. Effective limit resolves from an `account_limits`
  override row else a system-wide config default (`QUOTA_DEFAULTS`); current
  usage comes from COUNTs over the owning tables (the same counts the metering
  emit helpers use). A `quota.Wrap` middleware guards count-limited routes; the
  deploy and knowledge-connect handlers call `quota.Check` inline.
- **Consumption** — compute / knowledge storage stay on the existing entitlement
  path (billing provider) unchanged.

**Enforcement parity.** Over-limit blocking respects the same enforce flag as
before (`OPENMETER_ENFORCE`): when off, over-limit is logged, not blocked. A
disabled feature (effective limit 0) always 402s, matching the prior
"feature absent from plan" behavior. The 402 body shape and codes
(`FEATURE_NOT_IN_PLAN`, `ENTITLEMENT_LIMIT_REACHED`) are preserved verbatim.

**Storage.** New `account_limits` table (`account_id`, `resource`,
`limit_value`; 0 = disabled, -1 = unlimited). Only overridden pairs get a row;
everything else falls back to the config default. Admin-editable later via
astro-queen.

## Design — retire count events (1c)

With counts now authoritative in the DB, the resource-count events that only
existed to feed OpenMeter's count meters are removed: `active_agents`,
`active_deployments`, `active_members`, `active_knowledge_stores`,
`active_knowledge_endpoints`, and `agent_build`. Gone are their inline emit
calls (agents/org/deploy/knowledge handlers, the github-build worker), the
heartbeat's count-emit pass, and the `Emit*` helpers. Only metered-consumption
events remain: `compute_usage`, `knowledge_compute_usage`,
`knowledge_storage_provisioned` (startup `RequiredMeters` trimmed to match).

The usage endpoint (`GET /api/v1/accounts/:account/usage`) now sources
resource-count features from the quota DB via `quota.Reporter` and consumption
features from the billing provider — same response shape, authoritative counts.
Handlers/workers that only emitted count events drop their billing dependency.

## Migration

None required for existing behavior. No API changes; OpenMeter is still selected
implicitly by `OPENMETER_URL`, and metered events / 402 responses are unchanged.

`QUOTA_DEFAULTS` ships with the former private_beta plan limits (`agents=5`,
`agent_builds=50`, `agent_deployments=10`, `members=5`, `knowledge_stores=5`,
`knowledge_endpoints=2`). These are advisory until `OPENMETER_ENFORCE=true`
(over-limit is log-only otherwise), matching prior behavior. Override per
resource via `QUOTA_DEFAULTS` (e.g. `agents=10`) or per-account `account_limits`
rows (0 = disabled, -1 = unlimited). The `account_limits` table must be applied
from `sql/astro-server/schema.sql`.
