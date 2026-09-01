# Custom evaluator support: activation, resolution, and validation

## Summary

Persistence and document parsing for a builder-published evaluation set
landed separately with nothing calling them yet. This wires them in:
agents can now activate a custom evaluation set, and every evaluation
read/execution path resolves per agent instead of a single hardcoded
default.

## Design

A new resolver (`evalresolve`) is the one seam for turning an agent into
its evaluator list, used by every read and execution path instead of a
hardcoded default. A dedicated endpoint activates a custom set for an
agent independently of any build or registration, and a stateless
validate endpoint checks a document without persisting anything. The
validate endpoint is a stopgap: it exists only because astro-cli can't run
`evaldocument`'s checks locally yet, and should come out once that parser
is shared between the CLI and the server. Dataset item admission and the
review queue were also relaxed to key off a trace's own evaluation ref
instead of the agent's current one, so older results stay visible after
the active set changes.

## Migration

None required.
