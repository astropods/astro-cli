# Knowledge Stores Polish

## Summary

Visual pass on the Knowledge Stores list page and the "Add store" provider picker, plus a reusable `Table` primitive extracted from the Variables & Secrets page. Also refreshes and expands the integration icon catalog. No behavior changes.

## Design

**Reusable Table primitives.** Extracted the styled-table look used by the Variables & Secrets page into `components/ui/table.tsx` so other list surfaces can adopt the same row affordances, header treatment, and rounded wrapper without duplicating Tailwind. Fully on theme tokens (no ad-hoc stone colors), with Storybook coverage.

**Knowledge Stores list.** Empty-state CTA now uses the default button size to match the rest of the app, and the table treatment matches the traces table empty-state for consistency across "data lives here" surfaces.

**Provider selection.** Cards on the "Choose a provider" step are visually distinct from the stone-tinted shell (pure white in light mode, `dark:bg-popover` preserved for dark). Tightened spacing, the icon tile affordance, and the selected-state ring. Subtext clarified ("Pick the database or vector store to back this knowledge store"). The "Managed available" tag is now "Managed option" using the blue `Tag` variant, consistent with how optional capabilities are badged elsewhere.

**Integration icons.** Added Neo4j and MySQL brand marks and refreshed Qdrant, across light and dark variants.

## Migration

No migration required.
