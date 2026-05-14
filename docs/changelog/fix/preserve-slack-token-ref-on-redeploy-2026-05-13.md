# Preserve adapter-injected variable refs on redeploy

## Summary

Opening the configure panel for a deployment that originally bound a Slack
token (or any other adapter-injected variable) to an account variable
reference would show the secret field empty. The selected account variable
had been silently dropped — saving the form re-deployed the agent with no
token at all unless the user reselected it.

The bug was a timing problem in the deployment-template handler: stored
variables were merged into the template **before** the adapter-shaping pass
that injects `SLACK_BOT_TOKEN`/`SLACK_APP_TOKEN`/`SLACK_CONFIG`. Since the
keys didn't exist yet, the stored refs had nowhere to land.

## Design

The configure-redeploy flow has three layers of state:

1. **Base template** — derived from the published agent spec; cacheable per
   `(account, agent, build)`.
2. **Deployment overlay** — the values this deployment was last deployed
   with (adapter selection, ingestion schedules, authorization grants,
   variable refs/values, target identity).
3. **Request overlay** — in-flight edits the user typed into the configure
   form.

Previously, layer 2 mutated the template inline before `ShapeTemplate` ran,
while layer 3 went through the request pipeline. The two paths had
different ordering, and the variable subset of layer 2 was running before
adapter shaping could inject the variables it wanted to write to.

Layers are now routed consistently:

- **Variables** in layer 2 fold into `req.Variables` via a new
  `applyStoredVarsToRequest`, with user-typed values winning on conflict.
  `ShapeTemplate`'s existing variable-filling pass already runs *after*
  `ApplyAdapterShaping`, so adapter-injected keys exist by the time the
  stored ref lands.
- **Adapters, schedules, authz, identity** in layer 2 still mutate the
  template directly in `mergeDeploymentPrefill`. They don't have the same
  timing problem because `ApplyAdapterShaping` is happy to operate on a
  template whose adapter list is already set.

No duplicate `ApplyAdapterShaping` call, no inline mutation of variables.

`storedVars` are now fetched on every `deployment_id` request (cache hit
and miss) since they live in `req` rather than in the cached template. One
extra cheap DB query per cache hit; the cache still saves the
`generateTemplate` work, which is the expensive part.

## Migration

None. Existing deployments transparently get their refs back on the next
configure load.
