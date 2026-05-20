# App-wide loading UX

Addresses [#1083](https://github.com/astropods/astro/issues/1083). Branch name (`sohum/orgswitchingfix`) ended up narrower than the scope — what shipped is a unified loading-UX pattern across the authenticated surfaces (Agents, Blueprints, Knowledge, Insights) and the public account profile (`/:account` + Blueprints/Agents/Hearts tabs).

Followups deliberately punted to a separate PR: [#1089](https://github.com/astropods/astro/pull/1089) (`docs/01-spec/continued-loading-work-spec.md`) — scale strain points, mitigation order, and the agent-detail subpages we intentionally didn't touch.

## Summary

Loading state was a mix of structural skeletons and `initialData`-based hydration that flashed on every cross-key transition. Org switching in particular flashed because the active account lived in localStorage only — server loaders always fetched personal-account data while the client did the real work, so scope changes cache-missed every time. Profile navigation had the same shape: each `/:account` mount started with an empty cache, so tabs flickered.

The fix is a single pattern modeled on GitHub's repo navigation: a thin top progress bar covers the transition, the page stays mounted with the previous content, the new data is loaded server-side, then everything swaps atomically. Structural skeletons are removed wherever the pattern applies. Small inline skeletons (chips, sparkline cells, status pulses, sidebar stats) stay per team consensus that "large structural skeletons make pages feel perceptually slower."

## Design

**1. `NavigationProgressBar`.** Thin top-of-viewport sweep wired to `useNavigation` + `useRevalidator` state, mounted once in `Layout`. Visible whenever React Router is doing work (route navigation or programmatic revalidation), invisible otherwise. We deliberately do NOT watch `useIsFetching` — deployment polling keeps that count above zero forever, which got an earlier prototype stuck. The loader-state signal is the right proxy for "new page ready" because of the cache priming below.

**2. Cookie-backed active account.** `setActiveAccount` writes the `astro:active-account` cookie (with `Secure` on HTTPS) and calls `useRevalidator().revalidate()`. `getActiveAccount(request)` in `lib/api.server.ts` reads the cookie, validates it against the user's accounts, falls back personal → first account. `loadAccountScoped(request, fetch)` is the canonical loader shape — resolves the active account and returns `{ account, data }` so a typical page's loader is one line. Pages that need multiple parallel fetches (AgentDashboard) or extra inputs (Insights' `from`/`to`) inline their loader. The root loader also returns the resolved active account so `useActiveAccount` sources its initial value from there (not from `localStorage`) — fixes a hydration mismatch that flashed personal-account on hard refresh of an org page. A client override layer keeps `setActiveAccount` feeling instant; a one-time `useEffect` migrates legacy localStorage values to the cookie. Per-request `WeakMap` memo on `/me` so root + page loaders running in parallel only hit the backend once.

**3. Loader-driven cache priming via `usePrimeQueryCache`.** New hook that synchronously primes React Query during render (`useMemo`, not `useEffect` — `useEffect` runs after the page's `useQuery` hooks fire). Replaces the `initialData + initialDataUpdatedAt: 0` pattern that didn't anchor `placeholderData` on the first cross-key transition. Every adopted page uses it: AgentDashboard (deployments + all-time observability summary + account usage in parallel), Blueprints, KnowledgeStores (new loader added), Insights (activity + blueprints summary), AccountProfile (account + members).

**4. `placeholderData: keepPreviousData` app-wide.** Added to every account-scoped query factory that didn't have it: deployments (list, spec, history), blueprints (account list), knowledge (list, detail, metrics), accounts (members, orgs, account detail), github (account status + connections), slack (status), usage (account + quota), variables, hearts, and `useAccountObservabilitySummary` (the all-time summary used by DashboardStats). Skipped for sensitive endpoints (`useKnowledgeCredentials`, `useAuditLog`, `useAuditLogFilters`) — those would be a cross-tenant data leak even if only briefly visual during an org switch. The `initialData` option was removed from the 8 hooks that no longer had callers passing it, so future contributors don't accidentally reintroduce the flash.

**5. Insights specifics.** Range/agent toggles change search params but the data is keyed per `(account, from, to)`, so TanStack handles the fetch client-side. `shouldRevalidate` returns `false` for same-pathname search-param changes and `true` only for programmatic revalidation (the org-switch signal). The page resolves `from`/`to` preferring `loaderData` (the server-fetched timestamps the data is keyed under) and falling back to client `buildPeriodParams` for CSR range toggles — closes a UTC-day boundary mismatch that flashed a skeleton on first paint. Both values are passed into `useInsightsData` so the prime + lookup keys are always the same window.

**6. Structural skeletons removed.** AgentDashboard's deployment grid, Blueprints' card grid, KnowledgeStores' table rows, KnowledgeStoreDetail's full-page skeleton, BlueprintsTab/AgentsTab/HeartsTab in AccountProfile, plus the Insights subtree: `CostOverTimeChart` shimmer, `TopSpendersTable` ghost rows, `StatCards`/`MetricCard` value skeleton via dropped `loading` prop, `UsageCard` animate-pulse, `DashboardStats` `loading` plumbing. The single `AgentCardSkeleton` survives as the placeholder for the `LiveRevealOverlay` reveal animation (newly-deploying agent animating into its slot) — that's a reveal animation, not a loading indicator.

**7. Knowledge sub-pages migrated.** `KnowledgeStoreDetail` and `NewKnowledgeStore` moved from the legacy `useDefaultAccount` hook (localStorage-direct) to `useActiveAccount`. The legacy hook + its test are deleted (no remaining callers).

**8. Constants and utilities consolidated.** `ACTIVE_ACCOUNT_COOKIE`, `LEGACY_ACTIVE_ACCOUNT_STORAGE_KEY`, `readCookieValue`, and `OBSERVABILITY_WINDOW_ALL_TIME` all live in their domain modules (`lib/active-account.ts`, `api/queries/keys.ts`) so the server loader, client hook, and pages import one source of truth.

## SSR fix landed along the way

`DeployedAgentCard` was reading `window.location.origin` unguarded. It never triggered before because the dashboard's old loader returned only a count and SSR rendered no cards. With the new loader fanning out deployments + summary + usage in parallel and priming the cache, cards now SSR — exercised the latent bug. Guarded with `typeof window !== "undefined"`, matching the same construction in `LiveRevealOverlay`.

## Conventions for new pages

When adding an authenticated, account-scoped page:

**Loader.** Use `loadAccountScoped(request, (api, account) => api.listX(account))` from `lib/api.server.ts`. Returns `{ account, data }`. For pages that need multiple parallel fetches or extra inputs, inline the loader and call `getActiveAccount(request)` directly. Don't use `getPersonalAccount` unless the page is genuinely personal-scoped.

**Cache priming.** Use `usePrimeQueryCache(loaderData, (qc, ld) => …)` from `hooks/use-prime-query-cache.ts`. Do NOT use `initialData` on the query — it doesn't anchor `placeholderData` on the first cross-key transition and will flash the very first toggle.

**Query factory.** Account-scoped queries should set `placeholderData: keepPreviousData`. Exception: sensitive endpoints (credentials, audit logs, member PII) — those should fall back to empty rather than briefly show one org's data while another is in scope.

**Skeletons.** Don't add structural skeletons. The progress bar + `placeholderData` cover loading. Small inline skeletons (chips, sparkline cells) are fine. If you reach for a structural skeleton, the page needs cache priming instead.

**Active account.** Use `useActiveAccount()`. Don't read `localStorage` or `document.cookie` directly — the hook sources the initial value from the root loader (SSR-consistent).

**CSR-style toggles.** If the page has search params that should toggle data client-side without re-running the loader (range selectors, filters), add a `shouldRevalidate` that returns `false` for same-pathname URL changes and `true` for programmatic revalidations (`currentUrl === nextUrl` — the org-switch signal). See `pages/Insights.tsx` for the canonical example.

## Tests

New: `lib/api.server.test.ts` (`getActiveAccount` fallback chain, `loadAccountScoped`), `lib/active-account.test.ts` (`readCookieValue` edge cases including malformed encoding), `hooks/use-active-account.test.tsx` (override/SSR/personal precedence, cookie side effects, legacy migration), `hooks/use-prime-query-cache.test.tsx` (synchronous prime, re-prime on `loaderData` change, no re-prime on stable reference), `pages/Insights.test.tsx` (`shouldRevalidate` branches). Test loader stubs updated to match new shapes. `ActiveAccountProvider` moved inside the router stub in `test-utils.tsx` so `useRevalidator` has a data-router context.

Pre-existing test failures (`theme.test.tsx`, `experiments.test.tsx`, `blueprints/Blueprints.test.tsx`) — `localStorage is not a function` in vitest+jsdom — are unrelated and unchanged.

## Migration

None. No public API changes. `astro:active-account` cookie is written automatically on the next org switch; users who never re-switch fall back to their personal account. A one-time `useEffect` in `useActiveAccount` migrates any pre-existing `astro:default-account` localStorage value to the new cookie.

## Out of scope

- Stale Langfuse trace data from deleted agents (separate backend cleanup PR).
- Agent detail subpages (`/:account/agents/:deploymentId/{monitor,configure,deployments}`). Documented in [#1089](https://github.com/astropods/astro/pull/1089) as a focused followup — the layout still uses the pre-#1086 `initialData` pattern and Monitor/Deployments tabs render chart/table skeletons on first paint.
- Scale-related followups (hover-prefetch, `defer()` streaming, virtualization, pagination, backend perf on Langfuse summaries). Same spec PR.

Related work elsewhere: Chris's agent-card redesign and Jess's deploy-loader nav UX touch different surfaces; no overlap.
