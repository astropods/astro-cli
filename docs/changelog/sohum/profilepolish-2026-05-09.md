## Summary

A collection of polish fixes across the public profile pages — copy, color, layout, and empty states.

## Design

**Copy**
- "Edit photo" → "Change photo" in the settings page avatar dropdown
- "Go to profile" → "View profile" in profile settings
- "Remove photo" trash icon now matches the red destructive text color

**Layout**
- Website input prefix (`https://`) had double padding (~26px) due to `cn()` ordering; fixed by moving `px-0` after `inputBase` so tailwind-merge resolves correctly
- "View as visitor" toggle moved into the tab bar row (same line as tabs) to eliminate the large gap above the content

**Colors & fills**
- Outline button variant gains `bg-background` at the component level so the page grid/pattern no longer shows through (e.g. "Edit profile" button on the profile page)
- Early adopter badge gets a solid `var(--background)` SVG fill layer so the grid doesn't bleed through its semi-transparent gradient
- Early adopter account number `opacity: 0.6` removed for consistent color with the rest of the badge text
- Tab toolbar search input and dropdowns aligned to the same `--input-background` fill as the edit sidebar inputs
- "Customize order" button no longer turns purple when a custom order is saved — was an accidental `text-primary` override
- "Saved" confirmation button uses `var(--success)` green and drops the `disabled` prop (which was applying `opacity-35`) in favor of `pointer-events-none`

**Agents tab**
- "Only visible to you" label with `EyeOff` icon added inline in the toolbar, right-aligned, to indicate the tab is owner-only

**Reorder mode**
- Search input and filter dropdowns are now disabled (greyed out) during reorder mode instead of disappearing

**Org tooltip**
- `??` → `||` for `org.display_name` fallback so empty string display names correctly fall back to the org handle

**Empty states**
- `EmptyState` component gains a `card` variant (dashed border, icon, heading, description, action buttons) matching the dashboard pattern; `description`, `actionLabel`, `actionTo` are now optional; actions support icons and variants
- Blueprints tab uses the card variant with the `AgentMascots` illustration
- Owner empty state shows "No blueprints published yet." + "Create blueprint" (primary) + "Explore community" (outline) CTAs
- Visitor empty state shows "{DisplayName} has no public blueprints yet." (falls back to handle) + "Explore community" CTA
- Filter miss shows "No blueprints match your filters." with no CTA

**Hearts tab**
- "Date hearted" removed as a sort option; default is now "Most hearts"

## Migration

No action required.
