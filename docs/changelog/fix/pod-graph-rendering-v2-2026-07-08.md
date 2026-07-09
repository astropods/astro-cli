# Pod graph: deterministic columnar layout

## Summary

The deployment pod graph packed its tiles with a force simulation and drew
"constellation" lines between them via a minimum spanning tree. Both were purely
geometric: tile positions were emergent and seed-dependent, and the lines
connected nearest neighbors — which has no correlation with how the workloads
actually relate. Two unrelated databases could be linked; an ingestion job and
the store it populates usually weren't.

The graph also laid out once and never re-measured. Tiles change size after the
first pass (the async error-log row lands, status flips, age/warning rows
appear, fonts finish loading), so tiles overlapped or left gaps, and the
previous `layoutKey` workaround required the caller to enumerate every mode that
reshaped a tile.

## Design

A deployment is a hub: the agent is the one container everything else talks to.
The layout now reflects that structure, and every line means something.

- **Role classification.** Each tile's role (`agent`, `knowledge`, `model`,
  `integration`, `ingestion`, `collector`, `other`) is a pure function of its
  `component` label, with `kind` as a fallback for runtime-only ingestion
  entries. This is the single source of truth for both the tile icon and the
  layout.
- **Deterministic columnar layout.** The graph reads left-to-right as a data
  flow — `ingestion | knowledge | agent | others` — with each column a vertical
  stack centered on the vertical axis and the whole row centered horizontally,
  so the graph sits in the middle with the agent's inputs to the left and its
  models/tools/collector to the right. Positions are a pure function of role and
  measured size — the same input always produces the same layout, and a tile
  resizing only shifts its own column. No simulation, no warm-start, nothing to
  converge. Below 750px the columns collapse to a single role-ordered vertical
  stack.
- **Real relationship edges.** Edges follow the flow, derived from roles rather
  than proximity: each ingestion tile connects to *every* knowledge store
  (ingestion populates the stores as a group, so no single-store edge is
  implied), each knowledge store connects to the agent, and the agent connects
  to every model, tool, and the collector. Ingestion falls back to the agent
  when there is no knowledge to feed.
- **Agent avatar.** The agent tile renders the deployment's actual avatar in
  place of the generic agent icon.
- **Provider brand icons.** Knowledge stores and models render their provider's
  brand icon (Postgres, Redis, Qdrant, Ollama, …) when one is shipped. The match
  keys off the workload's declared `provider` — the platform's own identity for
  the container, not its user-chosen name — so a store named `mydb` running
  `provider: postgres` still gets the Postgres icon. `provider` is persisted on
  `deployment_workloads` at apply time and surfaced on the workload record; a
  one-shot River backfill worker fills it for pre-existing deployments from their
  stored spec. Anything unmatched falls back to the component name, then the
  generic role icon.
- **Continuous measurement.** A single `ResizeObserver` watches every tile and
  re-emits sizes from `borderBoxSize` whenever any tile's box changes, so a
  later size change drives a fresh layout instead of leaving stale positions.
  Tiles are positioned with CSS transforms (translate/scale), which don't affect
  the border box, so animating a tile never feeds back into the layout that
  placed it. `layoutKey` is gone.
- **Tile chrome.** The superellipse-masked "squircle" tile (and the
  `superellipsejs` dependency) is replaced by the shared `Card` primitive — a
  plain rounded rectangle on the standard border/`bg-card` tokens — keeping the
  hover tint and selected/dimmed states.

## Migration

Requires a schema migration, and it must land **before** the server code
deploys. The record endpoint's workload read (`GetWorkloadSummaries`) now
`SELECT`s a new `provider` column on `deployment_workloads`; if the code ships
before the column exists, that read errors and every deployment view degrades
(no workload detail, env, or URLs) until the column is applied.

The column is additive, nullable, and backward-compatible (older code ignores
it), so apply it first and independently: run **SQL Migrate (Prod)** (declarative
`atlas schema apply` from `schema.sql` — the diff adds
`deployment_workloads.provider`), then run **Deploy (Prod)**. The provider
backfill worker runs on startup and retries on failure, so it converges once the
column is present regardless of ordering; existing deployments show generic role
icons until it completes.
