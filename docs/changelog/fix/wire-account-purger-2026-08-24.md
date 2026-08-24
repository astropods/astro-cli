# Build the purge sequence where the admin server runs

## Summary

`AdminService.PurgeAccount` answered every request with `FailedPrecondition: account purge is not configured`, so an operator could delete a defunct account but never free its name ahead of the retention window.

The purger was taken from `Queue.AccountPurger()`, which returns the sweep's own `Purger`. Only the worker process builds one: the API process, where the admin gRPC server listens, opens an insert-only queue that registers no workers. The getter therefore returned nil in the only process that called it. `SetAccountPurger` ignores a nil argument, so the gap surfaced on each purge attempt rather than at boot.

## Design

The API process now builds its own `Purger`, next to the `Deleter` it already builds for the same reason. Both processes construct it through `accountlifecycle.NewPurger`, which is the single place that decides which optional collaborators a purge gets.

`NewPurger` takes the provisioners a caller owns (Langfuse, AI Gateway) and derives the stores each cleanup step reads from `DB`. A provisioner cannot arrive without its store, which is the pairing every revoke is guarded on:

```go
deps.Clients.AccountPurger = accountlifecycle.NewPurger(accountlifecycle.PurgerDeps{
    Log: log, DB: db, Deployments: deploymentStore, FGASync: deploymentFGASync,
    Langfuse: ingestLangfuseProvisioner, AIGateway: aiGatewayProvisioner,
    Undeploy: queue.UndeployFunc(deploymentStore),
})
```

The teardown hook is shared the same way. `Queue.UndeployFunc` returns the "set undeploying, then enqueue" step that a stalled deployment needs, so the two purgers re-enqueue teardown identically and an insert-only queue is enough to drive it.

Both pods read the same secrets and config, so the API process resolves the same backends the worker does and an on-demand purge cleans up what the sweep cleans up. `Queue.AccountPurger()` and the field behind it are gone, since a process that registers no purge worker has no purger to lend.

## Migration

None. The periodic sweep is unchanged.
