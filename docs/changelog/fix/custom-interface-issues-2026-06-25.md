# Fix: custom-interface public state lost on redeploy

## Summary

An agent whose custom (self-served) web interface was set to public — reachable
without an Astro account — silently reverted to protected when the deployment
was reconfigured/redeployed. The toggle would appear off again and the agent's
host would move back to the authenticated cohort on the next apply.

## Design

On redeploy the deployment-template is regenerated from the agent's base spec
and stored state is overlaid. The overlay copied the stored `interfaces.auth`
block (which carries the web/custom `public` flags) only when the freshly
generated template already had an `interfaces` block. A custom-interface-only
agent has no messaging block, so the freshly generated template had none, the
stored auth was dropped, and grant restoration (which only touches grant lists,
not the `public` boolean) couldn't recover it.

The prefill now creates an `interfaces` block when the stored spec has one, so
the stored auth — including `custom.public` — is restored. This mirrors the
deploy-time shaping path, which already synthesizes the block for custom-only
agents. Round-trip tests cover both `custom.public` and `web.public`.

## Migration

None. Affected agents simply need to be reconfigured/redeployed once on the
fixed server; the stored public state is now preserved.
