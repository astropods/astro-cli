## Summary

Org profile pages (`/{orgname}`) previously had a minimal ad-hoc layout. This PR replaces both `IndividualProfile` and `OrgProfile` with a single unified `AccountProfile` component that handles personal and org accounts through a shared layout tree. Along the way it eliminates the skeleton loader flash by SSR-seeding all data, fixes a mutation bug in the edit sidebar, restores Hearts SSR initialisation, and hardens two flaky tests.

## Design

### Component tree

```
AccountProfile (loader + data wiring)
└── ProfileLayout (visibility gating, sort, reorder, sidebar swap)
    ├── ProfileViewSidebar / ProfileEditSidebar  (sidebar slot)
    │   └── ProfileSidebarShell  (avatar/identity/meta/stats shell)
    └── BlueprintsTab / AgentsTab / HeartsTab   (main content)
```

`AccountProfile` is the single route component for `/:account`. It holds the React Router loader (SSR) and all TanStack Query wires. `ProfileLayout` is a pure presentational orchestrator — it receives the raw data and renders the correct tabs and sidebars; it knows nothing about fetching.

### SSR initialData

The loader runs six parallel fetches (account, orgs, members, blueprints, deployments, hearts) and passes every result as `initialData` to the corresponding TanStack query. Queries that receive initialData skip the client-side fetch entirely on first render, so no skeleton loaders are needed for the above data. `AccountProfile` sets `initialDataUpdatedAt: 0` on each query so a background revalidation still fires after mount.

### Unified sidebar

`ProfileSidebarShell` owns the avatar/identity block, edit button, bio, meta rows (date/pronouns/location/email/website), social links section, and stats grid. Both personal and org view sidebars are thin wrappers that pass their unique content (members list vs. orgs list) as `children`. The `ProfileEditSidebar` gained a `variant="org"` prop that swaps the display-name mutation, hides email/pronouns, and adjusts copy — eliminating the old `OrgEditSidebar`.

### Edit sidebar error handling

`handleSave` now runs both mutations in a `Promise.all` with a `try/catch`. On partial or full failure an inline error message appears below the Save button; `onClose` is only called on full success.

### Owner vs. visitor gating

`ProfileLayout` derives `visibleBlueprints` (hides private blueprints for visitors), the `Agents` tab (owner-only), the `Hearts` tab (personal accounts only), and the sidebar `isAdmin` flag all from a single `isSelf` boolean. The "View as visitor" toggle flips a `viewAsVisitor` state that overrides `isSelf` for the layout without re-fetching.

### Removed skeleton loaders

`AgentCardSkeleton`, `BlueprintCardSkeleton`, and the `loading` prop on `StatCell` are gone. Because SSR initialData is available immediately, there is nothing to show skeletons for on first paint. The sidebar stats animate in as regular content once the query resolves.

### Other fixes carried in this PR

- **Auth refresh bug**: `refresh()` / `refreshUserData()` calls in the edit sidebar were triggering blanket `QueryAuthSync` invalidation, flipping the page to visitor view mid-save. Removed; mutations now invalidate only the account detail query in `onSuccess`.
- **OrgSwitcher rename**: was invalidating with the wrong query key after an org renames; fixed to use the correct account query factory.
- **accounts.ts query hooks**: `useAccountOrgs` and `useAccountMembers` now accept an `enabled?` option so callers can conditionally disable them (e.g. disable orgs query for org accounts, disable members query for personal accounts).

## Migration

No action required.
