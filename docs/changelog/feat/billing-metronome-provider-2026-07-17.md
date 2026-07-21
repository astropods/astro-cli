# Trim the billing seam to metering-only; tighten the Metronome event schema

## Summary

Changes to the Metronome billing work: the `billing.BillingProvider` seam is
narrowed to metering-only (customer lifecycle + usage ingest), dropping the
balance/usage-query surface entirely; the emitted usage-event schema is
renamed for clarity and scoped down to deployment compute for now; the Metronome
Go SDK is upgraded to a pinned major version; and the settings **Usage** page is
replaced by a **Billing** page that renders Metronome's embeddable dashboards.

## Design

- **Seam is metering-only.** `CheckBalance` and `GetUsage` are removed from
  `BillingProvider`, along with the `Balance`, `UsageReport`, and `UsageItem`
  types. The interface is now `CreateCustomer` / `DeleteCustomer` / `IngestUsage`.
  astro-server no longer gates requests on balance, nor reads usage/cost back:
  gating is quota's job (DB-backed), and balances, spend caps, credit grants, and
  usage/cost reporting are provisioned and observed out-of-band (Metronome
  dashboard). This retires the "future `CheckBalance` gate" seam that the
  OpenMeter-removal change had left as a placeholder.

- **Event type renamed and scoped.** The compute usage event is now
  `deployment_compute_usage` (was `compute_usage`), and its metric property is
  `cu_hours` (was `compute_unit_hours`). The per-workload dimension is now the
  real `deployment_id` (was the K8s `namespace`, which is not a deployment
  identifier); it is sourced from `deployments.id` in every emit path. Knowledge
  metering (`knowledge_compute_usage`, `knowledge_storage_provisioned`) is
  disabled for now — the builder code stays in place but dormant (the heartbeat
  `Tick` and the knowledge handlers no longer call the emitters), so only
  `deployment_compute_usage` reaches Metronome.

- **Ingest schema documented as-built.** The implementation doc now carries an
  ingest event catalog: the exact envelope (`transaction_id`, `customer_id`,
  `event_type`, `timestamp`, `properties`), the emitted `deployment_compute_usage`
  payload, and the note that Metronome billable metrics must filter on the
  emitted `event_type` and aggregate on `cu_hours`.

- **SDK upgrade.** `github.com/Metronome-Industries/metronome-go` is pinned to
  `v3.9.0` (module path `.../metronome-go/v3`). The v3 API is source-compatible
  with our usage; only import paths changed.

- **Usage page → Billing page.** The settings **Usage** page (personal and org)
  is replaced by a native **Billing** page with tabs for **Usage**, **Invoices**,
  **Credits & Commits**, and **Quotas**. Rather than embedding Metronome's iframe,
  the server proxies Metronome's data APIs and the client renders it. The
  `billing.BillingProvider` seam gains `UsageData`, `Invoices`, and `Balances`,
  returning provider data verbatim (`V1.Usage.List` windowed by day,
  `V1.Customers.Invoices.List` with line items, `V1.Customers.Credits/Commits.List`);
  noop returns `ErrBillingUnavailable`. The endpoints
  `GET /accounts/:account/billing/{usage,invoices,balances}` return
  `{available, data}`; the API token stays server-side. Usage renders as a stacked
  bar chart + totals table grouped by whatever billable metrics the provider
  returns (no assumptions about metric names); invoices render as a native list
  (period, status, total) where clicking a row opens the invoice PDF in a modal
  (streamed through `GET /billing/invoices/:id/pdf`, which proxies Metronome's
  `Invoices.GetPdf` byte stream); balances render as tables derived from the
  returned fields. When the hosted
  backend is off (OSS/noop) or a customer can't be resolved, each tab shows a
  "not available" state instead of erroring.

- **Lazy customer provisioning.** Rather than a dedicated create endpoint, the
  billing customer is created on first billing access: if the account has no
  linked Metronome customer when the dashboard is fetched, the handler creates
  one and persists the linkage. This backfills accounts that predate billing.

- **Quotas fold into Billing.** The quota-increase flow that lived on the Usage
  page moves under a **Quotas** tab on the Billing page: a table of past requests
  plus a "Request increase" action (admins). The redundant per-feature usage
  tiles are dropped — Metronome's Usage dashboard is now the source of truth for
  consumption. Quota deep-links (deploy compute-limit, agent-limit) point at the
  Billing page.

## Migration

None for astro-server behavior on OSS/default (`noop`). For the hosted Metronome
backend, the dashboard-side billable metrics must match the emitted schema: the
compute metric filters `event_type = deployment_compute_usage` and sums
`cu_hours`; any group key previously on `namespace` should move to
`deployment_id`. Knowledge meters receive no events until re-enabled.
