---
title: Badge polish, PrivacyBadge updates, and archive action on blueprint detail
---

## Summary

A collection of visual polish fixes targeting badge consistency across the app — status badges, the privacy badge, and the archive action on blueprint detail pages. The main motivations were: PrivacyBadge felt too heavy, deployment status colors were inconsistent across surfaces, and the archive action on the blueprint detail header was visually prominent in a way that didn't fit the page hierarchy.

## Design

**PrivacyBadge** is restyled to use `text-muted-foreground` and `font-normal` with a stone border, making it feel like metadata rather than a warning.

**DeploymentStatusBadge** is consolidated — a duplicate component was removed and all surfaces now share a single source. Colors are normalized: deploying → yellow-600, inactive/undeploying → muted-foreground, active → existing green. `MonitorTab` trace status badges and `DeploymentHistoryRow` active badge are updated to match.

**BlueprintCard** fixes a name truncation bug where long names would collide with the kebab trigger. The kebab hover area uses `rounded-sm` to match the deployed agent card.

**BlueprintDetailHeader** moves the archive action from a standalone outline button in the header row into a kebab menu (⋯) that sits inline next to the blueprint name. This keeps the destructive action accessible to owners without it dominating the page chrome. The kebab only renders when `onArchive` is provided (i.e. the viewer is an owner), same gating as before.

**InlineBadge `variant="soft"`** now consistently applies `borderColor` across all usages.

**PageBreadcrumb** reduces the actions gap from the default to 8px for tighter grouping.

## Migration

No changes required.
