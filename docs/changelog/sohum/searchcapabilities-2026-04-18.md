---
## Summary

Redesigns the New Blueprint wizard's GitHub source step. The old flat dropdown for repo selection is replaced with a two-panel slide animation, an inline "connected" indicator, a searchable repo list with disabled states for already-linked repos, and a confirmation dialog before blueprint creation. The wizard code is also refactored into composable components with extracted named handlers.

## Design

### Two-panel slide

The source step renders a `200%`-wide flex container that slides left (`cubic-bezier(0.16,1,0.3,1)`) to reveal the repo picker when the user clicks "Select a repository":

- **Panel A** — source type selection. When GitHub is connected, a trigger button appears showing `{githubLogin} connected` (spinner until login loads) above a "Select a repository" button that shows the chosen repo name once picked.
- **Panel B** — repo picker (`RepoPicker` component): live search bar with a `githubLogin / repo-name` breadcrumb, scrollable repo list, private badges, disabled state for repos linked to other blueprints, and a branch selector that animates in below the list once a repo is chosen.

The card height animates from 460 → 600 px (`height` not `minHeight`, so the flex scroll constraint is maintained) when entering the repo picker panel.

The breadcrumb pattern (`sohum / credit-card-agent`) is intentionally designed to extend to `sohum / repo / sub-agent` for future monorepo support.

### Confirmation dialog

When the user clicks "Create blueprint" with a GitHub repo selected, a `LinkConfirmDialog` modal appears showing:
- A visual of the blueprint avatar connected to a GitHub icon
- Details table: visibility, org, branch
- "Back" and "Create blueprint" buttons

The actual create request fires only on dialog confirmation, preventing accidental blueprint creation.

### Disabled repo states

The repo picker fetches existing GitHub connections and disables any repo already linked to another blueprint, showing an inline "linked to {agent_name}" hint. This prevents double-linking.

### Component extraction and handler cleanup

`NewBlueprint.tsx` was refactored:
- Inline JSX handlers moved to named `useCallback` functions (`handleSelectGitHub`, `handleSelectLocal`, `handleOpenRepoPicker`, `handleRepoSearchChange`, `handleRepoSearchKeyDown`, `handleBack`, `handleSelectRepo`, `handleSelectBranch`, `handleCreateOrConfirm`, `handleConfirmAndPublish`)
- Repo picker UI extracted into `src/components/new-blueprint/RepoPicker.tsx`
- Confirmation dialog extracted into `src/components/new-blueprint/LinkConfirmDialog.tsx`

### Tests

- E2e tests in `github-onboarding.spec.ts` updated to the new UI: `connectGitHub()` waits for a "Select a repository" button instead of a combobox; `openRepoPicker()` clicks it and waits for the search input; test 2 (import flow) now handles the confirmation dialog step.
- Unit tests added for both extracted components (`RepoPicker.test.tsx`, `LinkConfirmDialog.test.tsx`, 25 tests total) covering loading state, empty states, repo list rendering, selection callbacks, disabled/linked repos, breadcrumb display, branch selector expand/collapse, and visibility toggling.
- Explicit `cleanup()` added to the global test `afterEach` to prevent portal elements from leaking between tests.

## Migration

No migration required.
