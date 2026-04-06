# Simplify dashboard header

## Summary

The dashboard hero section had redundant navigation — a breadcrumb bar with an org-switcher dropdown plus Browse Blueprints / Settings buttons that duplicated links already in the sidebar. This change removes the breadcrumb bar and action buttons, and replaces the dropdown-menu org picker with a compact inline Select control.

## Design

The OrgSwitcher now uses the existing Radix-based `Select` component (from `ui/select`) instead of a `DropdownMenu`. A "View" label sits beside it so the purpose is clear without the breadcrumb context. The scope switcher is placed inline in the hero row next to the greeting, keeping the header to a single visual block.

Action buttons (Browse Blueprints, Settings) were removed entirely since the sidebar already provides these links at all breakpoints.

## Migration

No migration required. No API or prop changes outside of the dashboard page.
