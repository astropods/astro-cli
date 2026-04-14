---
title: Archive action and blueprint card polish
---

## Summary

Moves the archive action on blueprint detail pages into a kebab menu and fixes several small UI issues on blueprint and deployed agent cards.

## Design

**BlueprintDetailHeader** replaces the standalone archive button with a kebab menu (⋯) inline next to the blueprint name. This keeps the destructive action accessible to owners without it dominating the page chrome. The kebab only renders when `onArchive` is provided (owner-gated, same as before).

**BlueprintCard** fixes a name truncation bug where long names would collide with the kebab trigger, and reduces the kebab hover area to `rounded-sm` to match deployed agent cards.

**PageBreadcrumb** tightens the actions gap to 8px. **ArchiveBlueprintDialog** uses `text-foreground` for the blueprint name in the confirmation copy.

## Migration

No changes required.
