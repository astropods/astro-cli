## Summary

The account profile page previously rendered blueprints as a flat list with no navigation between blueprints and agents. This adds a tabbed layout (Blueprints / Agents), a view-mode toggle so owners can preview their public profile as a visitor would see it, and client-side sort/filter controls for both tabs.

## Design

**View mode toggle.** Owners see an "Internal" vs "External" view. External mode hides private blueprints, the Agents tab, and the "Edit profile" button — matching exactly what a visitor sees. The toggle is a single `viewMode` state in `IndividualProfile`; all child components derive their behaviour from `isOwner` + `effectiveViewMode` props.

**Tab components.** `IndividualProfile` was split into three files:
- `BlueprintsTab` — search, visibility filter (owner-only), sort dropdown, and the blueprint card grid.
- `AgentsTab` — search, sort dropdown, and the deployment card list. Hidden for visitors.
- `TabToolbar` — shared `TabSearchInput` and generic `TabFilterDropdown<T>` used by both tabs.

All data fetching and filtering/sorting logic stays in `IndividualProfile`; the tab components are presentation-only.

**Blueprint sort.** The server returns blueprints alphabetically (`ORDER BY name`). The "Newest" sort (default) uses `max(versions[].published_at)` so the most recently published blueprint appears first. "Name A–Z" and "Most deployed" sorts are also available.

**Agent sort.** The server returns deployments by deploy time (`ORDER BY deployed_at DESC`). The default "Last modified" keeps server order; "Name A–Z" re-sorts client-side.

## Migration

No user action required. The profile URL (`/:account`) is unchanged.
