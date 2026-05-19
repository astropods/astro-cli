# Fix deployment validation for components with build config

## Summary

Deploying (or upgrading) an agent whose `astropods.yml` defines a component
(agent, model, knowledge, integration, or ingestion) with `container.build`
but no explicit `container.image` failed with `<component>.image is required`.
The source spec is valid — the deployment template generator just didn't
synthesize an image name for this case the way the build pipeline does.

## Design

The build pipeline (`TransformSpecForRegistry`) already converts
`container.build` entries into image references using a canonical naming
convention. That convention is now exposed from `packages/astro-spec` as a
single helper so every site shares one source of truth:

```go
func ComponentImageName(kind ComponentKind, agentName, name string) string
```

- `ComponentAgent`              → `{agentName}`
- `Component{Model,Knowledge,Integration,Ingestion}` → `{agentName}-{kind}-{name}`

The deployment template generator gains a local `resolveBuiltImage` wrapper
that synthesizes the image name when `Image` is empty but `Build` is set, then
calls `resolveImage`. All five component builders (agent, model, knowledge,
integration, ingestion) now go through it, closing the gap that previously
only existed for knowledge/integration/ingestion and would have hit model and
agent the moment someone declared them via `build` alone.

`CollectComponents` and `TransformSpecForRegistry` also call
`ComponentImageName` instead of inlining the format string in ten places, so
renaming a kind (or adding a new one) is now a single-file change.

## Migration

No migration required. Agents that previously required a workaround
(`container.image` alongside `container.build`) will now deploy without it.
