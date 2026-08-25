# Gate the eval judge on billing

## Summary

An account with its credits spent and no card on file could queue eval-dataset
evaluation runs, and every one spent real money at the AI gateway.

The judge invokes the gateway on a long-lived per-account key that shares the
account's gateway customer, so its usage bills exactly like a deployed agent's:
a run on preview billed $0.1746 of `LLM Usage` to the account it belonged to,
matching the gateway's ledger. Nothing gated it. The billing gate is applied at
eight call sites, deploy, redeploy, wake, restart, rollback, the messaging proxy,
ingestion, and chat, and the eval path was not among them. The only thing that
would eventually stop such an account was the gateway's own per-customer budget.

## Design

**The queue endpoint is gated like a deploy.** `POST /deployments/:id/dataset/
evaluations` refuses a suspended account with the same 402 body, reason, and
resolving action every other consuming path returns, so a banner and a refused
run cannot disagree about the fix.

**The worker checks again before invoking.** A River job outlives the request
that enqueued it, so an account suspended between enqueue and run would still
spend. Both eval workers resolve the dataset's account and stop there, recording
a durable failure that names billing rather than a generic run error, so the run
does not silently retry against a stopped account. The evaluation worker fails
the run it has already claimed rather than cancelling the job, so the owner sees
why it stopped instead of finding nothing.

**The worker's gate mirrors the HTTP one rather than inventing a policy.**
Observe mode allows and logs, and a failed status read allows, because no account
should stop working over a lookup failure. Both behaviours match the gate the
request path uses.

## Migration

None. A suspended account that already has runs queued will see them fail with
the billing message instead of consuming gateway spend.

The gate shares the status store that carries the never-suspend exemption, so an
exempt account is not refused a run either.
