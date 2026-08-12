# Deploying an agent bound to a knowledge store in a local environment

## Summary

Deploying an agent that binds a connected knowledge store failed in a local
environment with an AWS credential error:

```
resolve bound knowledge: knowledge "memory": failed to resolve credentials for
store "…": create decryptor: kms Decrypt: … get credentials: … InvalidGrantException
```

The message points at AWS, but AWS was never the problem: the request should not
have gone there at all. Connecting a store and deploying an agent were using
different KMS backends, so local development encrypted a store's data key with one
key and then tried to decrypt it with another.

## Design

`knowledgestore.KMSBackend` chooses a backend purely on environment: `ENVIRONMENT
== "local"` selects the compiled-in local backend, anything else real AWS KMS. The
connect path passes that flag, so connecting a store locally wraps its data key
with the local backend and needs no AWS access — which is why connecting succeeded
while deploying did not.

The deploy path never consulted it. `loadConfiguredKMSClient` branches only on
whether `KMSKeyARN` is set; locally it is not, so the deployer received a nil
client and its fallback built an AWS client unconditionally. Fixing AWS credentials
would not have helped, only changed the error: an AWS key cannot unwrap a blob the
local backend wrapped.

The fallback in `Deployer.kmsClient` now calls `knowledgestore.KMSBackend` with
`Cfg.Deployment.IsLocal()`, so both paths make the same choice from the same input.
That restores the invariant `KMSBackend` documents for itself — the same credential
logic runs everywhere, and only the backend behind the `envelope.KMSClient`
interface differs. An explicitly supplied `KMSClient` still wins, so the production
wiring is untouched.

Deciding by "is AWS reachable" rather than by environment is what made this
possible, and it is the kind of fallback that fails at the worst moment: the two
halves of an encrypt/decrypt pair have to agree, and only the environment knows
which half to use.

## Migration

None. Non-local environments set `KMS_KEY_ARN` and a non-local `ENVIRONMENT`, so
both paths already resolved to AWS KMS and behaviour there is unchanged. Local
environments can now deploy agents that bind a connected store; stores connected
locally before this fix remain readable, since they were wrapped with the local
backend all along.
