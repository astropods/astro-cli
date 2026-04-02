## Summary

Delivers the agent dashboard (`/dashboard`) as the primary post-login experience. The dashboard shows account-level observability stats, a filterable grid of deployed agents, and org-aware navigation. Reusable UI primitives (`MetricCard`, `MultiSelect`, `OrgSwitcher`) extracted during this work are available to other pages.

## Design

### Dashboard page (`AgentDashboard.tsx`)

The dashboard composes three focused child sections:

**`DashboardStats`** — Three metric cards showing total tokens (input + output), total requests, and compute usage for the last 24 hours. Tokens and requests display a percentage trend vs the prior 24-hour window, derived from two sequential calls to `GET /api/v1/accounts/:account/observability/summary`. Compute usage comes from `GET /api/v1/accounts/:account/usage`. Trends are suppressed when today's value is zero to avoid misleading "∞%" indicators on idle accounts.

**`DeployedAgentsSection`** — Fetches all deployments for the account and renders them as agent cards via `DeployedAgentCard`. Request counts are batch-fetched with `useObservabilitySummaries`. Filtering and sorting are owned by the `useAgentFilters` hook (pure, no API calls) which supports text search (name or display_name), status filter (multi-select), and sort by name / recent / requests. Active agents link to their monitor tab (`?tab=monitor`); deploying/errored agents link to the deployments tab.

**`DashboardToolbar`** — Filter input, status multi-select, and sort dropdown that bind to `useAgentFilters`'s toolbar props.

### Account context and org switching

The active account is carried in the URL as `?account=<name>`, defaulting to the personal account. `OrgSwitcher` renders in the breadcrumb and updates the search param. Personal account is sorted first in the dropdown regardless of the server-returned order.

Org accounts get a member count label (from `GET /api/v1/accounts/:account/members`) and suppress the Settings button. Personal accounts show Settings. The blueprints label links to the org-scoped blueprints page for org accounts and to the personal blueprints page otherwise.

### Server changes

`ListMembers` (`GET /api/v1/accounts/:account/members`) no longer requires `org:manage` — any authenticated member of the account can list members, which is needed to show the member count on the dashboard. The handler still enforces account membership. Mutating member routes (POST, PUT, DELETE) retain the `org:manage` requirement via a sub-group.

The route group in `main.go` was consolidated: a single `memberRoutes` group applies `ResolveAccount`; a nested `memberManageRoutes` sub-group adds `RequireAccountPermission(org:manage)` for write operations only.

### Navigation wiring

- `dashboardPath = "/dashboard"` constant added to `routes.ts` and used across `AppHeader`, `DeployBlueprint`, `DeployedAgentDetail`, `DeployedAgentSettings`, and `ActiveDetailView` — replacing ad-hoc string literals.
- `DeployBlueprint` redirects to `/dashboard` (or `/dashboard?account=<org>` for org deploys) instead of `/agents` after a successful deploy.
- Back navigation in org deployment detail now returns to `/dashboard?account=<org>` rather than the org profile page.

### Reusable primitives extracted

**`MetricCard`** — labeled metric value with optional trend indicator. Trend direction and color are driven by `higherIsBetter`.

**`MultiSelect`** — composable Radix Popover-based multi-select used by `DashboardToolbar` and `MonitorTab`.

**`OrgSwitcher`** — account switcher dropdown that reads from `useAuth().accounts`.

**`ComputeUsageCard`** — usage bar card showing consumption against quota with a CTA link.

### Tests

Unit tests cover `useAgentFilters` (text filter, status filter, all three sort modes, combined), `AgentDashboard` (agent cards, counts, filtering, stats values, trend direction), and `OrgSwitcher` behavior (personal account ordering, member count display, settings button visibility). E2e tests cover dashboard load, search filter, monitor tab links, and post-deploy redirect.

## Migration

No action required. The `/agents` page is unchanged. `DeployBlueprint` now redirects to `/dashboard` after a successful deploy instead of `/agents`.
