## Summary

Users deploying agents from blueprints had no visibility into build status or which build backs a deployment. This adds build tracking to two surfaces: the blueprint page's "Deployed agents" list and the agent deploy panel. Closes #1495.

## Design

**Blueprint "Deployed agents" list** (`SidebarDeployedAgents`). Each deployed instance now shows its short build ID, and the row's trailing slot is a single build-status column: a muted "Latest" tag when the instance is already on the newest published build, or an "Upgrade" button when it is behind. Upgrade links to that instance's configure flow targeting the latest build (`/{account}/agents/{id}/configure?build={latest}`), the same path the in-agent `UpgradeNudge` already uses, so no new deploy flow was introduced. Build ID and last-deployed time sit together on the sub-line for every row so the status column stays scannable, and the section's info tooltip explains that outdated instances can be upgraded.

**Agent deploy panel** (`DeploymentHistoryPanel`). A build-in-progress card renders above the active deployment whenever a GitHub build is in flight, driven by the existing `useGitHubStatus` query (which already polls while the latest build is pending or building). It mirrors the `DeploymentTile` layout: commit message as the title, a warning-tinted status pill with a spinner ("Preparing" while queued, "Building" while running), and the GitHub branch/sha line, so the two cards read as one family.

Long titles in both the in-progress card and `DeploymentTile` now truncate (`min-w-0` so flex `truncate` engages), keeping their status pills pinned right. The shared `SidebarSection` info tooltip is capped at `max-w-[260px]` so long copy wraps.

The ticket also suggested a deploy button on the blueprint page; one already exists ("Deploy this agent" in the sidebar, which spins up a new instance from the latest build), so no change was needed there.

## Migration

No action required.
