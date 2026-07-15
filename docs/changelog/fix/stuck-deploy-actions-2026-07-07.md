# feat: recover from a stuck deployment

## Summary

When a deployment got stuck in the deploying state, the recovery actions were disabled, leaving users with no recourse but to wait. This surfaces recovery up front and steers users to the action that actually fixes a stuck deploy. Closes #1573 and addresses the follow-up in #1584.

## Design

Detection is event-driven first, with a real-deploy-age timeout as a defensive backstop (per the review discussion on #1577).

- **Actions are never disabled.** Pause, Redeploy, and Restart stay enabled throughout a deploy (issue #1584), so a stalled deploy never leaves the user without a way to act. The stuck state is surfaced only on the page banner, not as a note inside the actions menu.
- **Event-driven detection (primary).** The server classifies each Kubernetes event via `humanizeDeploymentEvent`, which now returns a `severity` (`info` / `transient` / `stuck`) alongside the plain-language `title` + `guidance`, and covers the common stuck causes: failed scheduling, image-pull failures, and crash loops (the ambiguous `BackOff`/`Failed` reasons are disambiguated by message). While deploying, the client reads the deployment events and, when a `stuck`-severity event is present, raises the banner immediately and names the cause (its `title` + `guidance`). This is the primary trigger, so a real problem surfaces as soon as Kubernetes reports it, not on a fixed timer.
- **Real-deploy-age timeout (defensive).** For a hang with no clear event, the status endpoint now returns `status_changed_at` (already stamped server-side on every status transition). The client marks the deploy stuck once `now - status_changed_at` exceeds the threshold (5 minutes). Measuring from the server timestamp, not page load, means it is correct across reloads and for a user returning to an already-stuck deploy, which the previous mount-timer got wrong.
- **Recovery.** The banner leads with "Roll back" (the last clean version is usually the fastest fix) with "Pause" alongside, so it has exactly two CTAs, and the body carries a "Why deploys get stuck" docs link. The last good version is the highest-revision past deploy that is not current and did not fail; with no earlier version the banner falls back to Pause as the primary action. Built from the shared `ActionPanel` (which gained an optional secondary action), below the tab bar and agent identity header, width-constrained for readability.
- **Docs.** The "Why deploys get stuck" link points to a new "Troubleshooting stuck deployments" page listing the common causes and how to recover.

Remaining backend follow-ups from #1577/#1584: the build-failure headline ("Build failed: ...") and an LLM-generated cause for arbitrary failures with no humanized event.

## Migration

None. This is additive.
