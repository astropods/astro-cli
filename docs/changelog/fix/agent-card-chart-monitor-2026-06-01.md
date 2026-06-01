# Agent card on `/agents` opens Monitor; Manage button still opens default detail

## Summary

On the `/agents` page, clicking anywhere on a deployed agent card now opens that deployment's Monitor tab. The "Manage agent" button on each card is unchanged — it still routes through `AgentDetailRedirect` to the default detail panel.

## Design

`DeployedAgentCard` was already a click target — the whole card has an `onClick`/`onKeyDown` that navigates to a single `detailPath`. Previously that path was `deploymentPath(account, deploymentId)` (which redirects to `/deployments`); now it's `/${account}/agents/${deploymentId}/monitor` directly.

The sparkline is the card's headline visual and is observability data, so Monitor is the natural expanded view for the card as a whole. Routing at the card level (rather than wrapping the chart in its own `<Link>`) keeps the markup flat, preserves the existing keyboard/focus affordances on the card, and avoids nested-link semantics.

Two deliberate boundaries:

- **Manage button untouched.** The Cog "Manage agent" button continues to point at `deploymentPath(account, deploymentId)`. `eventStartedFromCardInteractive` already filters card-level clicks that started inside a button/link, so the button's destination is preserved even though the card around it now navigates somewhere else.
- **Cards without a `deploymentId` stay non-interactive.** `detailPath` is `undefined` when no id is available, which short-circuits both `handleCardClick` and `handleCardKeyDown` and drops the `role="link"` / `tabIndex` from the wrapper. No broken `/agents/undefined/monitor` URLs.

As a follow-on effect of `deploymentPath`'s new default tab argument, `ConfigureDeployment`'s post-submit `navigate(basePath)` (`apps/astro-client/src/pages/configure/ConfigureDeployment.tsx` lines 28 and 81) is a third call site that now lands directly on `/{account}/agents/{id}/deployments` instead of bouncing through `AgentDetailRedirect` — mirroring the same client-side hop already skipped for the Manage button.

## Migration

None required.
