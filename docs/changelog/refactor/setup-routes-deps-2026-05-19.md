# Group setupRoutes dependencies into a Deps struct

## Summary
`setupRoutes` in `apps/astro-server/main.go` had grown to 29 positional
parameters. Adding a new shared dependency (store, client, etc.) required
threading it through `main` → `runAPI` → `setupRoutes` and updating the
call site, making the wiring layer brittle and PRs noisy. The function
body itself was fine — the problem was the seam.

## Design
A new `Deps` struct in `apps/astro-server/deps.go` bundles every
dependency `setupRoutes` consumes, organized into two sub-structs:

- `Stores` — persistence-layer types (`account`, `deployment`, `audit`, …)
- `Clients` — external/infra clients (`k8s`, `loki`, `openmeter`, `queue`, …)

Top-level fields (`Log`, `Cfg`, `DB`, `Ent`, `Probe`) stay flat because
they are referenced by nearly every route group.

`runAPI` constructs the `*Deps` once after its existing initialization
block and passes it to `setupRoutes(router, deps)`. The route
registration body is unchanged — a block of local aliases
(`accountStore := deps.Stores.Account`, etc.) at the top of the function
preserves every existing reference, keeping the diff contained.

Handler signatures are intentionally untouched; they can migrate to
accept `*Deps` (or a narrower per-domain struct) incrementally when
those files are next modified.

## Migration
None. Internal refactor; no public API or behavior change.
