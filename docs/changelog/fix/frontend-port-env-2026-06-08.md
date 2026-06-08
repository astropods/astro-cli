## Summary

Frontend agents (`agent.interfaces.frontend: true`) crash-looped behind the ingress unless the agent's code hardcoded the correct port. The platform routes traffic to `:80` in production and to the configured dev port locally, but never told the container which port to listen on — most frameworks read `process.env.PORT`/`PORT` and fall back to their own default (Express 3000, FastAPI/uvicorn 8000, Next 3000) when it's missing.

This PR closes the gap by injecting `PORT` into the agent container in both deployment paths.

## Design

The injection is symmetric across the two builders that assemble the agent's environment:

- **astro-server** (`internal/deployment/template.go`, `GenerateDeploymentTemplate`) — when `astroSpec.Agent.HasFrontend()` is true, set `agentEnv["PORT"] = "80"`. Production frontends are pinned to `:80` by spec validation rule 15, so the value is constant.
- **astro-cli** (`internal/compose/builder.go`, `BuildEnvironment`) — when `s.Agent.HasFrontend()` is true, resolve to `dev.interfaces.frontend.port` when set, otherwise `"80"`. Matches the existing port-publishing logic at the compose level so the value the agent binds to is the value the compose project routes to.

Both injectors only fire when `frontend: true` — non-frontend agents continue to receive no `PORT`, since they only speak the messaging gRPC protocol and have no HTTP listener to bind.

## Migration

None. New env var on frontend agents only. If your agent already hardcodes `app.listen(80)` it keeps working; if it reads `process.env.PORT` it now gets the right value instead of falling back. Locally, restarting `ast project start` after pulling picks up the change automatically.
