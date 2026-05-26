## Summary

Allows the new `trace-router` component in `monitoring` to deliver LLM-proxy spans to each agent's per-namespace collector via OTLP. Without this, the per-agent fan-out used by the per-namespace Langfuse routing design (see `modules/astro-infra/docs/plans/per-namespace-langfuse-routing.md`) is blocked by the tenant namespace's `default-deny-all` baseline.

## Design

The tenant-side `allow-namespace-traffic` NetworkPolicy already carried one ingress rule for the `monitoring` namespace — opening port 9091 so Alloy could scrape the messaging sidecar metrics endpoint. The same monitoring → tenant pattern applies to the LLM trace path:

- `trace-router` (otelcol-contrib Deployment in `monitoring`) ingests Envoy proxy spans, enriches them with `k8sattributes`, and uses its routing connector to fan the same span out to (a) Tempo and (b) the originating agent's `<agent>-collector.<ns>.svc:4317` so Langfuse picks it up via the existing per-agent pipeline.
- The cross-namespace OTLP send requires explicit ingress from `monitoring` to the collector pods. Rather than introduce a separate NetworkPolicy (or a Kyverno GeneratingPolicy that watches collector Services), the existing monitoring ingress rule just gets two more ports:

```go
Ports: []networkingv1.NetworkPolicyPort{
    {Protocol: ProtocolTCP, Port: 9091}, // Alloy scrape (existing)
    {Protocol: ProtocolTCP, Port: 4317}, // OTLP gRPC
    {Protocol: ProtocolTCP, Port: 4318}, // OTLP HTTP
},
```

This keeps a single owner (`astro-server`) for tenant NPs, avoids a new policy surface, and matches the existing convention of `monitoring → tenant` access being a narrow port allowlist.

## Migration

No action required for new agent deployments — `applyNetworkPolicies` runs on every deploy and writes the updated NP.

**Existing live agents keep their stale NP until their next deploy.** If platform operators want the new ports active without a redeploy wave, run:

```bash
kubectl get ns -l name=monitoring -o name >/dev/null  # sanity check
for ns in $(kubectl get ns -o jsonpath='{range .items[?(@.metadata.name)]}{.metadata.name}{"\n"}{end}' | grep '^astro-'); do
  kubectl patch networkpolicy allow-namespace-traffic -n "$ns" --type=json -p='[...]'
done
```

(In practice we redeploy each agent — the patch is only needed if there's a real urgency.)
