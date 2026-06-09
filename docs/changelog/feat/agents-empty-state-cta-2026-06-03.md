# Agents page empty state: route to "Deploy a blueprint" when the account already has blueprints

## Summary

On `/agents`, the empty-state CTA used to always say "Create blueprint" — even when the currently-selected account already had blueprints that could just be deployed. That's the wrong primary action for a user whose only missing step is to deploy something they've already authored. The empty state now branches on whether the selected account has blueprints and routes accordingly.

## Design

The `DashboardAgentsEmptyState` component now takes the selected `account` as a prop and consults `useAccountBlueprints(account)` to decide which CTA to render:

- **Account has ≥1 blueprint** → primary action becomes **"Deploy a blueprint"** linking to `blueprintsAccountPath(account)` (the account-scoped blueprints list, where each card has a Deploy affordance). Copy: "Pick one of your blueprints and deploy it to get an agent running."
- **Account has no blueprints** → unchanged behavior: **"Create blueprint"** → `/getting-started`, with the existing "you'll need a blueprint" copy.

The secondary "Explore community" action is unchanged in both branches.

The query (`useAccountBlueprints`) is the same cache entry used by the `/blueprints` page, so navigating to `/agents` after creating or deleting a blueprint generally hits a warm cache. On a cold first load the empty state initially renders the "Create blueprint" path and flips to "Deploy" once the query resolves — a brief, visually mild flash that we accepted rather than block the empty state on a second query.

## Migration

None required.
