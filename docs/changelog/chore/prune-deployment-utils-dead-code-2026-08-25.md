# Summary

Three exports in the client's `deployment-utils` module had no callers in the product. Two
of them stayed invisible to dead-code detection because their own test file imported them,
which is enough to make an export look used. This removes all three.

# Design

`isLaunchReady` gated the Launch button on a strict record-level check: the messaging URL
exists, the sidecar is in the spec, and the endpoint reports ready. Both of its call sites
dropped it on purpose. The grid stopped using it when Launch moved to rendering whenever a
messaging URL is set, because the host in the ingress rule is final as soon as the deploy
applies and only the load balancer's status lags. Gating on `ready` therefore hid a button
that already worked. The agent card dropped it separately.

Leaving the helper in place is not neutral. It encodes a gate the current Launch paths
rejected, so it reads as available for reuse and invites a regression back to the behavior
those changes removed. Its doc comment goes with it.

`launchUnavailableMessage` was referenced only from inside the `isLaunchReady` tests, where
it filled a field no assertion read. `formatDaysActive` dates from the first agent dashboard
and lost its last caller when the agent card was rewritten.

The audit covered all 20 exports in the module. The other 17 have live callers and are
unchanged.

Two things stay deliberately. `PAUSED_DEPLOYMENT_RECORD_STATUSES` and its companion type are
exported but consumed only inside the module: over-exported, not orphaned. The `ready` and
`message` fields on `ServiceEndpointInfo` lose their last client readers here, but the
server still populates them, so the interface continues to mirror the API contract.

# Migration

No user action required. The removed symbols were unreachable from the product.
