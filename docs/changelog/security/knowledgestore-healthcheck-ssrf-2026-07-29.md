# Block SSRF to internal addresses in the knowledge-store health check

## Summary

The knowledge-store connect flow health-checks the store before saving it, and
`CheckHealth` dialed the user-supplied `HOST`/`PORT` for every provider with no
restriction on the destination. Because astro-server has unrestricted egress and
the success/failure result is returned to the caller, any account member could
use the endpoint as a blind reachability and port-scan oracle against internal
infrastructure — RDS, Redis, cloud metadata, and the pod network.

## Design

The guard lives at the dial layer rather than in a pre-flight validation of the
submitted host. Validating the hostname up front is inherently racy: a name that
resolves to a public address when checked can resolve to a private one when the
connection is actually made (DNS rebinding). Instead, a single shared
`net.Dialer` carries a `Control` hook that inspects the *resolved* IP at dial
time, immediately before the socket is connected, which closes that window by
construction.

`isPublicIP` rejects anything not globally routable: loopback, unspecified,
link-local unicast (covering `169.254.169.254` cloud metadata), multicast, and
RFC1918/IPv6-ULA private ranges. It also rejects CGNAT `100.64.0.0/10`, which
Go's `IsPrivate` does not cover and which is the cluster pod-network range — so
the health check cannot be aimed at in-cluster pod IPs.

The remaining work was routing every provider through that one dialer, since
each client library exposes a different seam:

- Postgres — `pgx.ParseConfig` plus `config.DialFunc`, replacing `pgx.Connect`.
- MySQL — the driver only dials through registered net names, so the guarded
  dialer is registered as `tcp-ssrf-guarded` and `cfg.Net` points at it.
- Redis — the `Dialer` option on the client.
- HTTP — a dedicated client whose transport uses the guarded dialer, with no
  proxy configured; an HTTP proxy would tunnel the connection and bypass the IP
  check entirely.
- Raw TCP — the guarded dialer directly.

Two seams are package vars (`ipAllowed`, `healthHTTPClient`) purely so tests can
reach loopback test servers; production never reassigns them.

## Migration

No user action required for stores reached over the public internet. Stores that
are only reachable at a private address will now fail the connect health check —
use PrivateLink (which bypasses the check) or `SkipHealthCheck`.
