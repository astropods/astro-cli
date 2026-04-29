# Knowledge Store Polish

## Summary

Visual polish across the knowledge store creation flow, detail page, and settings. Restores two post-create screens dropped in a prior refactor, redesigns the provisioning stage with a step list, and aligns the detail page and settings cards with the agent settings design system. Also fixes dark mode `destructive` token visibility.

## Design

**Post-create flow.** `SuccessStage` was recreated from the pre-polish version when `NewKnowledgeStore.tsx` was split into atomic components, dropping all changes from PR #753. Restored: confetti, animated check icon, combined YAML+CLI code block card, and single-row back/view actions.

`PendingAcceptanceStage` (PrivateLink post-create screen) was dropped entirely in the same refactor. Restored: three-step stepper (Registered → Awaiting approval → Connected), store card, and `PrivateLinkSection` for instructions/events. The redundant "Action required" banner is suppressed on this screen since the stepper already communicates state. `PrivateLinkSection` gains a `showBanner` prop and stacks endpoint/region values instead of inline pills.

**Provisioning stage.** Replaced the raw K8s event log with a fixed step list showing all stages upfront — completed steps with a teal check, active step with a spinner, upcoming steps dimmed to 35% opacity. Labels match the server's `humanizeKnowledgeEvent` output. Header centered with accurate copy.

**Provider selection.** Removed the "Managed option" tag from provider cards.

**Detail page overview.** Stone-200 headers on Agent bindings and Event log cards, `rounded-md` corners, white fill on metric cards with dark mode variant. Chip dark mode fix. Ghost "View logs" button. LogViewer toolbar gets stone-200 header.

**Settings panel.** Aligned to agent settings style: stone-200 headers, `rounded-md`, `divide-y` row separators with no body background. Danger Zone redesigned with `destructive` token for border/header and a new `variant="inline"` on `DangerZoneItem` to render as a plain row inside the parent card.

**Theme.** Dark mode `destructive` token updated from `red-800` to `red-400` — `red-800` was nearly invisible on dark backgrounds. All usages now auto-adapt without per-component overrides.

## Migration

No migration required.
