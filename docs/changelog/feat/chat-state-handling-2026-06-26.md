## Summary

The chat composer only had a binary enabled/disabled with a generic "Agent is not active yet" line, and the inspector's Settings tab fetched the agent's `agent/config` unconditionally. The latter hit the messaging proxy even when the agent's sidecar was unresponsive, where the proxy (which has no upstream timeout) hung ~60s and returned 502 — tripping the `AstroServerHigh5xxRateByRoute` alert on `/messaging/agent/config`. This adds real agent-state handling and stops that 5xx source.

## Design

A single `deriveChatComposerState(summary, status, runtime)` helper maps the deployment's coarse status value + reason + live `messaging_reachable` to one state: `unknown | ready | paused | error | starting | stopped | unreachable`. The chat thread renders per state:
- ready (and unknown, while status loads) — normal composer, kept optimistic so a healthy agent doesn't flicker disabled on first paint.
- starting / unreachable — a short notice (spinner for starting) above a disabled composer, since these are transient.
- paused / stopped / error — the composer is replaced by a banner (icon + reason + "Open agent" link) and the thread is dimmed, signalling the agent is off.

The inspector's Settings tab derives the same state but gates the `agent/config` request pessimistically: it only fires once **both** the status and runtime queries have *settled* **and** the agent is `ready`. While either is still loading the state is optimistically `ready`/`unknown` for the composer, but the Settings gate treats it as not-yet-known and shows "Checking agent status…" rather than issuing the request — closing the first-render window where the request could fire against a paused/unreachable agent. A runtime *error* counts as settled (not still-loading), so a persistently failing runtime read doesn't pin the tab on "Checking…" for an otherwise-healthy agent. The query itself is hardened: `retry: false` and `refetchOnWindowFocus: false` (so a failure isn't multiplied), and a ~10s abort timeout so a hung sidecar fails fast instead of holding the connection.

Chat eligibility (`messaging_web_configured`) is enforced at the page level — `useChatAgents` only surfaces web-configured deployments and the chat route redirects/empty-states anything else — so the thread and inspector only ever mount for an eligible deployment. `deriveChatComposerState` therefore takes only `(status, runtime)`; the now-unused eligibility predicate was removed rather than left as dead code.

`messaging_reachable` is now an actual sidecar-readiness signal, not bare Service presence. The runtime endpoint previously set it from "the messaging Service object exists", which stays true even when the sidecar is crashed/wedged — the exact case that hangs the proxy. It now additionally requires the messaging sidecar **container** (a native sidecar in the agent pod, whose readiness already surfaces via merged init-container statuses) to be `Ready` when it appears in the live pod view, falling back to Service presence only when no container has surfaced yet. This means the client derives `unreachable` for a wedged sidecar and never attempts `agent/config` — removing the 504 (which still counts as 5xx) at its source for that scenario rather than just shortening it.

Defense in depth on the server: the messaging proxy still applies a 15s upstream deadline to non-stream requests (SSE streams stay unbounded), so any remaining unresponsive sidecar returns a prompt 504 rather than hanging until the client disconnects.

Coverage: `deriveChatComposerState` (all states incl. the loading/unknown and active-but-unreachable cases) and the server-side `messagingSidecarReadiness` helper (present-and-ready, present-but-wedged, absent → fall back) now have unit tests.

## Known limitation

This change removes the dominant source of `/messaging/agent/config` 5xx (the client no longer calls a wedged sidecar, and the remaining server path fails fast with a 504 instead of a ~60s hang), but a 504 still counts toward `AstroServerHigh5xxRateByRoute`. If a sidecar flaps badly enough to still trip the alert, the alert definition itself (e.g. excluding 504 or this low-volume route) needs tuning in `astro-infra` — that is intentionally out of scope for this PR.

## Migration

No action required.
