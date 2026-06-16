## Summary

Polish pass across the Settings tabs (Account, Organizations, Experiments) and the shared header primitive. No functional changes; the Connectors redesign lands separately.

## Design

**Unified header pattern across every tab.** `SectionHeader` now owns the entire header treatment (title, subtitle, optional action slot, divider, bottom gap) via `pb-4 mb-4 border-b border-border` plus a flex header row with an `action` slot for trailing buttons. Tabs that previously hand-rolled their own `h2 + Separator` (Organizations, Members, Secrets, Audit Log) now use the primitive.

**Account tab.** Field layout polished and the Danger Zone gets a `pt-8` section break (with a `TriangleAlert` + mono eyebrow heading and inline `hr`) so the destructive area reads as a distinct sub-section without needing a full SectionHeader. Heading hierarchy corrected so the Account header stays at `h2` and Danger Zone is `h3`.

**Organizations tab.** Org rows are now fully clickable `<Link>` cards with a trailing chevron — the previous pattern had a clickable name with a separate role badge. Each row uses `border border-border rounded-lg px-4 py-3` and a `space-y-3` container.

**Experiments tab.** Per-experiment toggles drop their per-row `bg-card` chrome — the row borders already separate from the page surface and the extra elevation read as double-layered. Tab also adopts the shared `SectionHeader` instead of an inline header.

## Migration

No action required.
