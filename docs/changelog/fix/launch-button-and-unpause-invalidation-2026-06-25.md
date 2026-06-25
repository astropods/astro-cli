## Summary

Two user-visible defects on the agent surfaces:

1. **Launch button (disabled state)** — when an agent isn't running, the disabled Launch button rendered its trailing arrow on a second line, and the "why is this disabled" tooltip overflowed the viewport edge with a clipped descender.
2. **Stale status after unpause** — resuming a paused agent left the status toggle/badge stuck on "Paused" until a manual refresh.

## Design

**Launch button.** The disabled branch wrapped its label in a plain `<span>`, so the `Button`'s flex layout (`inline-flex items-center gap-2`) never reached the text and icon and the arrow wrapped. Replacing the wrapper with a fragment lets the disabled `<button>` lay out the label and arrow as direct flex children — identical to the enabled `<Link>` branch (where `asChild` merges those classes onto the link). The disabled tooltip gained a `max-w` so long messages wrap, `collisionPadding` so it never touches the viewport edge, and a little extra vertical padding so descenders aren't clipped. The agent-detail Launch button, previously a hand-rolled `<Link>` with ad-hoc primary styling, now uses the shared `Button` primitive so both Launch buttons render consistently.

**Unpause invalidation.** The root cause is that deployment status is **not monotonic across a resume**, and `useDeploymentStatus` was treating a single terminal read as final. The wakeup handler flips the DB status to `pending` *synchronously* before its `202` (the status endpoint maps `pending`/`provisioning` to `"deploying"`, a polling state) and a worker then drives it to `active` — so the request does *not* ack before the status leaves "stopped". The stuck-on-Paused regression instead comes from a server-side race *after* the worker reports `active`: the reconcile worker can observe KEDA's `ScaledObject` still reporting `Active=False` during cold start (the freshly-woken pods haven't taken traffic yet) and re-mark the namespace `scaled_down`, which the status endpoint maps back to `"inactive"` — a terminal, non-polling state. The old `onSuccess` invalidated immediately and `useDeploymentStatus` stopped polling the moment it read `"inactive"`, so the UI stuck on "Paused" until a manual refresh. (Pause didn't hit this because the server reflects "undeploying" instantly and monotonically.)

The fix makes the client resilient to non-monotonic status rather than assuming the server only moves forward:

- **Optimistic in-progress transition** — the list entry is set to `pending` (keeps `useDeployments` polling) and the status query to `deploying` (starts `useDeploymentStatus` polling), so the UI reflects the resume immediately.
- **Resume grace window (sliding)** — `useDeploymentStatus` polling is no longer terminated by a single non-polling read. `markDeploymentResuming(id)` opens a module-level window (`statusRefetchInterval`) during which interim `"inactive"` reads keep polling at 3s. The window *slides*: every transitional (`deploying`/`provisioning`) read pushes its deadline forward rather than counting down from `onSuccess`, so a slow cold start (image pull, KEDA scale-from-zero) that runs past the per-read slack can't lapse the window mid-warmup and strand a later `"inactive"`. The window closes early once status converges on `"active"`, or lapses if two consecutive reads go a full slack apart without a transitional read. Module-level so every observer of the status key honors it — `DeploymentTile` and `DeploymentHistoryPanel`, which read only `useDeploymentStatus` with no intent flag, recover purely through this window.

The existing runtime flip-to-active effect still sweeps the detail subtree (record + URLs) once the server reports ready.

Note: the server-side reconcile/KEDA race that produces the transient `scaled_down` is the underlying cause and could also be addressed there (e.g. a cool-down before re-marking a just-woken namespace). This change hardens the client so the badge converges regardless of that timing.

## Migration

No action required.
