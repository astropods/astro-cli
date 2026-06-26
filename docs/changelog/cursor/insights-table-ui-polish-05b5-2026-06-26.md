# Insights Table UI Polish

## Summary

The insights table had three visual inconsistencies that reduced readability and wasted space: a "Rank" label that repeated obvious information, name text sized smaller than other values, and rank numbers right-aligned away from their column. These polishes tighten the layout and improve visual hierarchy.

## Design

### Remove Rank Label

The table header previously showed "Rank" above the rank numbers (1, 2, 3...). Since the numbers are self-explanatory in a sorted list context, the label was removed. The column remains (preserving layout structure), but the header cell is now empty. This eliminates awkward whitespace and shifts all content left. To maintain accessibility, the `RankMarker` component includes an `aria-label` (e.g., "Rank 1") so screen readers announce the context without reintroducing the visible label.

### Unify Text Sizing

Identity labels (people and agent names) previously used `text-body-sm`, making them noticeably smaller than metric values. Changed to `text-body` semantic token to match the rest of the table, creating consistent visual weight across all columns.

### Left-Align Rank Numbers

Rank numbers were right-aligned within their column, creating visual distance from the "Name" header and content below. Changed to left-align by removing `text-right` from the `RankMarker` component, aligning ranks directly with the name column for cleaner scanning.

### System Icon Cleanup

System identity avatars displayed a Server icon inside a circular `bg-muted` container, inconsistent with how other identity types render. Removed the `bg-muted` background while maintaining proper alignment by wrapping the Server icon in a span that applies `baseClassName` (which incorporates the caller's size props). This ensures the icon occupies the same box width as other avatars (20px when passed `size-5`), so labels align properly across all identity types. The icon itself is sized at `size-3.5` with `strokeWidth={1.75}` to match other fallback icons visually.

## Migration

No action required. These are visual-only changes with no API, data model, or behavior modifications.
