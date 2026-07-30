# Server-authoritative chat turn termination

## Summary

A chat turn could end without the client ever being told. If the agent crashed
or its gRPC stream dropped mid-turn, the messaging sidecar tore down the stream
handler and told nobody: the SSE clients kept receiving 30s heartbeats with no
terminal event, and the conversation derived `assistant_streaming = true` until
the pod restarted. The only thing that ever ended such a turn was a client-side
timer, which guessed "stalled" from the gap between streamed chunks and so
tripped on healthy-but-slow turns (the "response timed out" error shown over a
reply that was still streaming).

The fix makes the **server authoritative for turn termination** and demotes the
client timer to a pure transport backstop.

## Design

Termination now has a single owner per failure mode:

- **Agent disconnect / crash.** When the agent's stream handler returns (a crash
  sends FIN/RST, so `Recv` errors promptly), the server notifies adapters via the
  new optional `AgentDisconnectHandler`; the web adapter finalizes every in-flight
  turn (terminal error event + `FinalizeTerminal` in the store + close the
  conversation's SSE connections).

- **Hung agent (connected but silent).** The web adapter's turn tracker gained a
  per-turn idle watchdog, armed when the user's message is forwarded (so a turn
  that produces no output at all is still reaped) and reset on any agent activity.
  On expiry it finalizes the turn the same way as a disconnect.

- **Clean finish / typed agent error / user stop.** Unchanged; already
  server-driven.

All abnormal terminations funnel through one `failTurn` helper, and the tracker
claims a turn atomically (`failActive`) so exactly one terminal event fires even
when the idle timer, a finish, a stop, and a disconnect race.

**Client.** The SSE transport now listens for `heartbeat`/`status`/`error`
events. A server `error` event ends the turn immediately and shows the server's
message. The client watchdog is now a transport-liveness check: it resets on any
inbound event (chunks *and* 30s heartbeats) and fires only after 90s of total
silence, meaning the pipe itself is dead (proxy/sidecar/network), the one case
only the client can observe. It no longer guesses about the agent.

## Migration

None. No config or API changes. The turn idle window defaults to 5 minutes
(`WithTurnIdleTimeout` overrides it).

## Server component and sequencing

The server-authoritative half described above (`AgentDisconnectHandler`, the turn
tracker's idle watchdog, and the `failTurn`/`failActive` single-owner
termination) lives in the **messaging sidecar** repo, not this monorepo: see
astropods/messaging#74. This client change relies on it — the client demotes its
timer to a pure transport backstop and depends on the sidecar's terminal `error`
event to reap a hung-but-heartbeating agent.

So messaging#74 must merge and the `modules/messaging` submodule be advanced to a
commit that includes it **before (or together with)** this PR. Shipping this
client change against an older sidecar regresses the hung-but-heartbeating agent
case: heartbeats keep the 90s liveness watchdog alive, so the turn is only ended
by the 15-minute content-stall cap instead of the old 3-minute timer (bounded,
not indefinite, but a worse worst case). This PR does not itself bump the
submodule.
