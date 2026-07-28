# Persistent page filters

## Summary

List and analytics filters now survive navigation, so returning to a page restores the working view instead of resetting it.

## Design

Each account-aware page stores its filters under a page-specific browser storage key. Agents and Blueprints preserve account selections; Knowledge Stores preserve account; Insights preserves account, range, view, and source selections. Search text and sort order reset when their page unmounts.

URL-backed account and Insights filters remain shareable. Explicit URL params replace their matching stored values while omitted filters retain their prior filter selections; an unfiltered page visit restores those selections without removing unrelated callback parameters. Search terms remain ordinary URL or component state and are not restored from browser storage.

URL-backed filters restore from browser storage after mount, apply local writes directly, and subscribe only to storage clears to avoid storage/URL feedback loops. Clearing the shared storage resets every mounted filter when the authenticated session changes account, user, or organization, or signs out.

Filtered-empty Agent views keep the toolbar visible, so restored filters remain apparent and editable even when the selected accounts contain no matching deployments.

## Migration

No migration is required.
