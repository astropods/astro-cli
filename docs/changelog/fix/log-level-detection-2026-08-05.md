# Summary

Error logs were invisible in the deployment log UI. Not missing — collected,
stored, and returned, but never identifiable as errors. Two independent defects
in how a log line's level is resolved, plus a paging default that hid recent
lines from the CLI.

This came out of debugging a Slack agent that stopped replying: the sidecar was
logging the reason, and no view in the product could surface it.

# Design

**Level resolution.** Our Alloy pipeline deliberately sets no `level` label and
leaves level detection to Loki 3.x, which attaches `detected_level` as
structured metadata at ingest. Two places assumed a `level` label existed:

- `QueryLogs` read `detected_level` only from stream labels, where it appears
  only if the query promotes it with `| keep` (which `TailLogs` does and
  `QueryLogs` did not). Every entry on the paged path therefore came back with
  an empty level. It now also reads `detected_level` from the per-entry
  structured metadata that `query_range` already returns.
- `LevelFilter` emitted `| level = "<value>"`, a label filter on a label that
  does not exist, so it matched nothing. Every "most recent error" lookup
  returned empty, which is what blanked the per-container error indicators. It
  now filters `detected_level`.

**Unknown is not INFO.** `normalizeLevel` maps a missing level to `INFO`, so
level-less entries rendered as INFO and the error/warning filters counted zero
of them. Log views and the filter hook now resolve a level with `entryLevel`,
which falls back to a keyword scan of the message (the same heuristic
astro-server already applies to raw pod logs) and otherwise reports `UNKNOWN`,
rendered as a blank badge. `normalizeLevel` itself is unchanged.

**Paging direction.** `direction` selects which slice of the time window is
returned, not the order of the returned entries. The Loki path always returned
entries oldest-first; the K8s fallback reversed them for `backward`, which
contradicted both the Loki path and the client's cursor (it pages off
`entries[0]`). The reversal is gone, so every page is oldest-first regardless of
backend.

`ast agent logs` (astro-cli) relied on the server defaults, `direction=forward`
and 200 lines, so it printed the *oldest* 200 lines of the last 15 minutes. On a
sidecar logging at debug level that is a few seconds of traffic from a quarter
of an hour ago, never the event being investigated. It now asks for the most
recent 500.

# Migration

None. No API or query-parameter changes; callers that passed `level` keep
working and now get results.
