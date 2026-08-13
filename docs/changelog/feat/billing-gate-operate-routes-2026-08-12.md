# Gate the deployment-operate routes on billing

## Summary

A suspended account could still restart pods, restart a whole deployment, roll
back to another revision, and start an ingestion job. Only deploy and wakeup
refused. The suspension scaled workloads to zero, so these requests either fought
that or consumed compute the account was not paying for.

## Design

The gate cannot be route middleware here. `Entitlements.Wrap` reads the account
from the gin context, and these routes are deployment-scoped: the account comes
from the deployment record, which is only known after the lookup. So the check
stays inline, and the four routes now share one helper with deploy and wakeup:

```go
if blockedByBilling(c, entCheck, dep.AccountID) {
    return
}
```

Placement is the whole point of the helper, and its doc comment records both
constraints. It runs after the membership check, because the 402 names the
account's billing state and a non-member must not read it. It runs before any
precondition check, so a billing stop is reported as a billing stop rather than
as a bad request.

### Chat and messaging already refused, with the wrong message

`forwardChat` and `ProxyDeploymentMessaging` both return
`404 "chat endpoint unavailable"` for a deployment that is not active, and a
billing-suspended deployment is not active. So nobody could send a message to a
suspended agent, but the answer read as an outage rather than a billing stop.
Both now carry the gate ahead of that status check, so the cause is named.

**Writes only.** Conversation history lives in the deployment's own database, so
it is unreachable while the workload is scaled to zero. That is an outage, not a
policy, and gating the GET would turn it into one: the read would stay refused
the day history outlives the pod. The messaging path allowlist is per-character
rather than per-route, so that route carries reads as well (agent config,
conversation fetches, the SSE stream), and those stay open too.

A suspended account therefore cannot reach its chat history today. That is a
data-availability consequence of scaling the workload to zero, and it is worth
deciding on separately from billing.

### Not gated

Undeploy, delete, stop, and cancel all reduce usage. Refusing them traps spend
rather than stopping it, and a suspended account still has to be able to clean
up. Deploy-validate touches nothing.

Image push never reaches astro-server. astro-registry terminates `PUT /v2/*path`
against its own database connection, so gating it belongs in that service.

## Security fix

`RollbackDeployment` checked the deployment's status and current revision before
it checked membership. Any authenticated caller holding a deployment id could
read another tenant's deployment state out of the error text: "can only rollback
active or failed deployments" or "already on this revision". Membership now
decides first.

`TestRollbackDeployment_WrongStatus` passed only because of that ordering: it
queued no membership row and short-circuited on the status check. It now grants
membership and asserts the status refusal as a member.
