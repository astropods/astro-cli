# Connectors settings card layout

## Summary

Improve the connectors settings page hierarchy so available services and their connected accounts are easier to scan as one related group.

## Design

Connectors now share a single bordered surface with roomy primary rows and design-system icon tiles. Connected GitHub organizations and Slack workspaces appear in inset detail panels, preserving their relationship to the parent connector while keeping account-level actions visually prominent.

The layout uses existing Astro AI surface, border, typography, spacing, and control tokens. Connector headers use scope-aware success summaries: GitHub shows its connected account handle, while Slack summarizes the number of connected workspaces. Slack account handles appear as muted inline metadata beside their individual workspace names.

When GitHub has no approved organizations, its access guidance remains directly on the parent surface instead of introducing an empty nested panel.

Descriptions can wrap on narrow screens so connection details and actions remain readable without truncation.

## Migration

No migration is required.
