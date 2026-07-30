# Block path traversal in the messaging reverse proxy

## Summary

`ProxyDeploymentMessaging` appended the caller-supplied wildcard path tail to
the upstream URL without normalization. Outside dev, that upstream is the
Kubernetes API-server service-proxy subresource, reached with a client carrying
astro-server's own cluster credentials. A traversal sequence that survived into
the upstream path could climb out of the intended
`/namespaces/<ns>/services/<svc>/proxy/api/` prefix and reach another tenant's
service or arbitrary kube-API endpoints as astro-server's service account —
cross-tenant SSRF escalating to cluster compromise. Relying on the edge WAF was
not sufficient, since double-encoding (`%252e%252e%252f`) bypasses it.

## Design

The defense is an anchored charset allowlist, `^[A-Za-z0-9/_-]+$`, applied to
the raw path before the upstream URL is constructed.

An allowlist rather than `path.Clean` plus a `..` rejection, because
sanitization here is decode-order dependent: whatever normalization runs, an
attacker gets to choose how many encoding layers sit between the edge and this
check. Excluding `.` makes a `..` segment unrepresentable, and excluding `%`
means there is no percent-encoding left to smuggle any encoded variant through.
Correctness no longer depends on how many times the path was decoded upstream.

This is viable because the messaging web adapter's route surface is fixed and
narrow: `POST /api/conversations`,
`/api/conversations/{uuid}/{messages,stream,history,audio}`,
`GET /api/agent/config`, and `/health`. Only alphanumerics, `/`, `-`, and `_`
are ever legitimate. The check fails closed, so a future endpoint needing other
characters must extend the pattern explicitly. Query parameters — which do
legitimately use `%`, `=`, and `&` — travel in `RawQuery` and are unaffected.

The upstream host itself is server-constructed from the authz-checked
deployment record and never derived from the request path, so the path is the
only attacker-influenced component.

## Migration

No user action required.
