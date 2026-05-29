# Fix prod smoke test: deployment-active assertion

## Summary

The `app.post-deploy` smoke test had been failing in prod since the V2 agent card
shipped (PR #1170). The test polled `/agents` for a card containing both an
`<a>` wrapper and the literal text "Active" — neither of which the redesigned
card surfaces.

## Design

The V2 card on `/agents` no longer wraps in `<a>` (it's a `<div>` with inner
Links) and only surfaces error / update-available status pills — "Active" is
intentionally implicit. The canonical "Active" status string still renders on
the agent detail page via `AgentStatusToggle`, which carries
`data-testid="agent-status-toggle"`.

The smoke test now reads `deploymentSlug` from the CLI state file already
populated by `app.deploy.spec.ts`, navigates to
`/${username}/agents/${deploymentSlug}`, and polls the status-toggle testid
for the "Active" label (same 14-minute window as before).

This shifts the assertion from a brittle text-on-grid match to the explicit
status surface, which also matches what an operator would check on the
detail page.

## Migration

None.
