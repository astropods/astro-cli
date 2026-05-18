# Fix deployment validation for knowledge containers with build config

## Summary

Deploying (or upgrading) an agent whose `astropods.yml` defines a knowledge
entry with `container.build` but no explicit `container.image` fails with
`knowledge.<name>.image is required`. The source spec is valid — the deployment
template generator just doesn't synthesize an image name for this case the way
the build pipeline does.

## Design

The build pipeline (`TransformSpecForRegistry`) already converts
`container.build` entries into image references using the naming convention
`{agentName}-knowledge-{name}`. The deployment template generator now applies
the same logic: when `container.Image` is empty and `container.Build` is
present, it synthesizes the image name before passing it to `resolveImage()`.

The fix covers all three container types that can use `build`:

- **Knowledge:** `{agentName}-knowledge-{name}`
- **Integrations:** `{agentName}-integration-{name}`
- **Ingestion:** `{agentName}-ingestion-{name}`

Each builder function now accepts the entry name so it can construct the
synthetic image reference.

## Migration

No migration required. Agents that previously required a workaround
(`container.image` alongside `container.build`) will now deploy without it.
