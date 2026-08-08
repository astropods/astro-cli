# Network flow graph on the agent monitor page

## Summary

The Network Traffic section showed aggregates and a flat table — the numbers, but not the shape of an agent's traffic. This adds a bilateral graph leading the section: inbound routes on the left, outbound destinations on the right, sized by request volume and connected to a central agent tile. The flows table below picks up matching brand icons.

## Design

The graph draws both directions at once, so it can't ride the table's direction toggle. It issues its own inbound and outbound flow queries with no `sort` or `limit`, which makes their keys identical to the table's on the matching tab — TanStack Query serves that one from cache, so the net cost is one extra request. Outbound hosts merge into one bubble per vendor (`api.`/`hooks.`/`files.slack.com` become one `slack.com`, with the constituent hosts listed on hover and capped at five); latency percentiles are dropped rather than combined, since p95s across hosts don't sum into anything meaningful. A bubble wears a brand icon only when one resolves against the shipped icon manifest — everything else carries text fitted to the circle, because a wall of identical globes distinguishes nothing, and inbound peers are route templates with no brand at all.

Grouping needs the registrable domain, and that boundary is registry policy rather than anything derivable from a hostname: `bbc.co.uk` and `mail.google.com` are the same shape with different answers, and `github.io` is itself a public suffix, so `alice.github.io` and `bob.github.io` are unrelated parties. Rather than ship a public suffix list to the browser — measured at +43KB gzipped — the flows endpoint now returns `registrable_domain` per address peer, computed with `golang.org/x/net/publicsuffix`, which was already a dependency. The client groups on that field and treats an empty one as "stands alone", so bare IPs and single-label internal names never merge. The list stays current through ordinary dependency bumps instead of hand maintenance.

## Migration

Nothing required; the graph hides itself when an agent has no traffic or its container is too narrow. Traffic metrics come from in-cluster instrumentation, so a locally-run agent shows an empty panel.
