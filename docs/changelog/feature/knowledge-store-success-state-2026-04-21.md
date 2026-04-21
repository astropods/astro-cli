## Summary

The post-create screens for knowledge stores — success and PrivateLink pending-acceptance — were visually rough and didn't use the design system consistently. This redesigns both into polished, information-dense confirmation screens that match the quality bar of the rest of the product.

## Design

**SuccessStage** gets a celebratory treatment: a confetti burst, an animated check icon (spring scale-in + stroke-draw path animation via `ks-pop` and `ks-check-draw` keyframes), a store card with provider icon/mode Tag/Ready StatusBadge, and a YAML + CLI snippet card with copy buttons and slim scrollable code containers. Actions are a single inline row — ghost back link on the left, primary CTA on the right.

**PendingAcceptanceStage** (new) handles PrivateLink stores that require manual endpoint approval in the user's cloud console. It renders as two cards: a store card (provider icon, mode Tag, spinning Pending badge) and a 3-step vertical card list:
- Step 1 (complete): light teal check circle, "Store registered in Astro," with endpoint ID and region rendered as inline `bg-stone-100` pills that wrap and truncate-safe on narrow viewports
- Step 2 (active): yellow numbered circle, cloud-specific title and description, outline CTA button linking directly to the relevant cloud console
- Step 3 (locked): faint numbered circle, "Astro verifies your connection — happens automatically"

The store card uses a responsive `flex-col sm:flex-row` layout so the mode tag and status badge stack below the name on mobile. Cloud console config (`CLOUD_CONSOLE`) drives step 2 per provider (AWS/GCP/Azure) with distinct titles, descriptions, and deep-link URLs. The `ProvisioningStage` store card is also updated to match — same provider icon, mode Tag, and spinning Pending badge.

Both screens include confetti via `LiveRevealConfetti`, scoped to the content container via `containerRef` so pieces traverse the visible area rather than the full viewport. A `speed` prop (set to `2` on knowledge store pages) scales velocity and gravity independently of other usages. The `ProvisioningStage` early-returns into `PendingAcceptanceStage` when `status === "pending-acceptance"`.

## Migration

No migration required.
