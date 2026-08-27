# Frontend Loading States

## The failure mode this prevents

A component that destructures `isLoading` from a query hook but never checks
`isError` or `isLoadingError` renders a failed fetch exactly like an empty or
"no data yet" state. The user sees a blank list, an empty form, or "Unknown
user" and has no way to tell a real outage from there being nothing there.
In a destructive flow (leaving an org, deleting a resource) or a form with a
save action, this is worse than a cosmetic gap: the UI can look safe to act
on when the data it based that on never loaded.

Always destructure the error signal alongside `isLoading` and give it its
own render branch.

## `isError` vs `isLoadingError`

TanStack Query exposes both:

- **`isLoadingError`** — the initial fetch failed and there is no data to
  fall back on. Use this when you want a background refetch failure to leave
  existing data on screen instead of tearing down a view that already has
  real numbers.
- **`isError`** — true for any failed fetch, including a background refetch
  that failed after data had already loaded. Use this when stale data would
  be misleading or the component has no "existing data" to preserve (small
  inline displays, tabs that reset per selection).

Pick one deliberately per hook call. Don't check neither.

## The pattern (`SettingsShared.tsx`)

`apps/astro-client/src/components/settings/SettingsShared.tsx` exports the
building blocks for a full section or page:

- **`LoadingRows`** — skeleton placeholder shaped like the content, shown
  while `isLoading`.
- **`LoadError`** — an error panel with a retry button, shown on
  `isError`/`isLoadingError`. Distinct from `Unavailable`: a load failure
  (network error, 5xx) is not the same as the feature being unavailable for
  this account, and conflating them tells a user mid-outage that a feature
  is off.
- **`Unavailable`** — the request succeeded but the data says the feature
  doesn't apply to this account (an `EmptyState` with a fixed message).
- **`EmptyState`** — the request succeeded and returned zero items, with a
  custom message.

```tsx
const { data, isLoading, isLoadingError, refetch } = useBillingSpend(account);

if (isLoading) return <LoadingRows />;
if (isLoadingError) return <LoadError onRetry={() => refetch()} />;
if (!data?.available || !data.data) return <Unavailable />;
```

Check the branches in this order: loading, then error, then "unavailable" or
empty, then the real content. An error must never fall through to the empty
or unavailable branch.

## Small, non-page components

A small inline component (a badge, a count, a one-line status) doesn't need
the full `SettingsShared` treatment, but it still needs to keep failure and
"genuinely nothing here" visibly distinct:

```tsx
const { data, isLoading, isError } = useAccountMembers(account);
const label = isLoading ? "…" : isError ? "Couldn't load" : "Unknown user";
```

Use judgment on presentation (a muted label is enough for a badge; a bigger
surface may warrant a retry action), but never let an error and an empty
result render the same text.

## Known instances not yet fixed

An audit found the same missing-`isError` shape at the call sites below.
Fix them opportunistically when you're already touching that file, rather
than as a standalone cleanup pass.

**Real risk** (data correctness or safety exposure, worth prioritizing):

- `apps/astro-client/src/components/settings/ManageLimitsDialog.tsx` (~line
  162) — `useBillingSpend` destructures `{ data, isLoading }` only; a failed
  load renders an empty, fully-editable limits form with Save enabled.
- `apps/astro-client/src/components/settings/LeaveOrganizationDialog.tsx`
  (~line 43) — `useAccountMembers` failure makes `members` empty, so
  `isSoleMember` becomes wrongly `true` in the "leave org" flow.
- `apps/astro-client/src/components/UserBadge.tsx` (~line 30) — a failed
  member-list fetch renders "Unknown user," indistinguishable from a
  genuinely unknown user.
- `apps/astro-client/src/components/agent-detail/pods/PodDetailPanel.tsx`,
  `EventsTab` (~line 364) — omits `isError` while the sibling `AlertsTab` in
  the same file (~line 404) handles it; `EventsTab` should match it.
- `apps/astro-client/src/pages/settings/ConnectorsSettings.tsx` (~lines 84,
  254, 464) — the GitHub, Slack, and Supabase connection-status hooks all
  lack `isError`; failure hides an active integration behind a "Connect"
  button on a security-relevant settings page.
- `apps/astro-client/src/pages/agent-detail/AgentConfigure.tsx` (~line 40) —
  the blueprint-versions hook feeds the redeploy/rollback form; failure
  looks like "no other versions to roll back to."
- `apps/astro-client/src/pages/AgentDetail.tsx` (~line 64) and
  `apps/astro-client/src/pages/knowledge/KnowledgeStoreDetail/KnowledgeStoreDetail.tsx`
  (~line 44) — both collapse a fetch error into a false "not found" page,
  which can read as the resource having been deleted.
- `apps/astro-client/src/components/deploy/grants/MemberPicker.tsx` (~line
  26) — the access-grant member picker shows "No members" on fetch failure;
  it fails closed, but silently.

**Cosmetic** (a list or badge that goes harmlessly blank on failure; left as
lower priority):

- `apps/astro-client/src/components/activity/TopSpendersTable.tsx` (~line 847)
- `apps/astro-client/src/components/blueprint-detail/SidebarDeployedAgents.tsx` (~line 35)
- `apps/astro-client/src/components/chat/ChatEmptyState.tsx` (~line 75)
- `apps/astro-client/src/components/new-blueprint/RepoPicker.tsx` (~line 37)
- `apps/astro-client/src/components/chat/ChatInspectorPanel.tsx` (~line 682)
- `apps/astro-client/src/components/agent-detail/evals/dataset/DatasetTable.tsx` (~line 94)
- `apps/astro-client/src/pages/AccountProfile/AccountProfile.tsx` (~line 61)
- `apps/astro-client/src/pages/AccountProfile/HeartsTab.tsx` (~line 19)
- `apps/astro-client/src/pages/agent-detail/AgentMonitor.tsx` (~line 95)
- `apps/astro-client/src/pages/agent-detail/AgentTraces.tsx` (~line 66)
- `apps/astro-client/src/pages/NewBlueprint.tsx` (~line 199)
- `apps/astro-client/src/components/agent-detail/pods/PodLogsTab.tsx` (~line 89)
