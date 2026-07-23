# Queen admin: flat nav and account detail view

## Summary

The admin console (Queen) collapsed to a single "Admin" section, so the
accordion sidebar wrapped one group for no reason. Separately, the accounts page
was a single dense, horizontally-scrolling table that tried to be both a browse
list and an edit surface — and it surfaced almost none of the account facts an
admin actually needs. Billing status and per-account resource limits, in
particular, are enforced by the platform but were invisible: there was nowhere to
see whether an account is suspended or what its effective limits are.

## Design

**Flat sidebar.** The nav is now a single flat, ordered link list. The
`TreeSection` accordion, its open/close state, and the chevron are gone;
collapse-to-icons and external-link handling are preserved.

**Accounts master → detail.** The accounts page splits into a scannable list and
a per-account detail workspace at `/admin/accounts/:id`.

- The list drops inline rename/cluster-migrate/actions and becomes triage-only:
  identity, a billing-status badge, cluster, members, Langfuse, status, created.
  Rows navigate into the detail page.
- The detail page is organized into cards, each the home for one concern:
  billing (status, dunning, and hosted-billing / card-on-file / Bifrost linkage
  flags), cluster placement (with the migrate action), resource usage & limits,
  pending quota requests (approve/deny inline), members with emails, and cache
  maintenance.

**New aggregate read.** A `GetAccount` admin gRPC endpoint returns the account
plus its billing status, per-resource usage and effective limits, and member
roster in one call. Usage and limits reuse the existing quota reporter
(`quota.Report`) rather than re-deriving counts, so the numbers match what
enforcement sees. The account list query also gains a `billing_status` column
(a left join on `account_billing_status`) to drive the list badge.

Because the proto package has no working codegen, the `.proto`, `admin.pb.go`,
and `admin_grpc.pb.go` were hand-edited in lockstep (JSON-over-gRPC codec).

## Migration

None. No schema changes; `GetAccount` is additive and reads existing tables.
