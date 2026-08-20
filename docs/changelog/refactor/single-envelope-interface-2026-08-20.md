# One vault for envelope encryption

## Summary

Seven places built a KMS client, and they disagreed about what a missing one
means. `knowledgestore.KMSBackend` picked the local dev backend outside AWS.
`loadConfiguredKMSClient` returned nil when `KMS_KEY_ARN` was empty. The AI
Gateway's `loadKMSClient` returned nil when the AWS config failed to load, and
its caller then stored the API key in the clear. Four more sites called
`kms.NewFromConfig` inline. The "is this encrypted?" test differed to match:
`keyARN == ""`, `kmsClient == nil`, `len(encryptedDataKey) == 0`, or `localMode`.

Underneath, `Encryptor.Encrypt` returned the plaintext as ciphertext with a nil
nonce when it had no key, so any caller could write a plaintext row that read
back cleanly. That is the mechanism that split `account_langfuse` into two
populations nobody noticed until two readers disagreed. A census of the other
nine envelope-backed tables found no drift, so this change closes the hole
rather than repairing damage.

## Design

`envelope.Vault` owns the backend and the key ARN, and is the only way to reach
encryption:

```go
vault, err := envelope.Open(ctx, cfg.Deployment.IsLocal(), cfg.Deployment.KMSKeyARN)

enc, err := vault.Encryptor(ctx)                       // fresh data key
enc, err := vault.EncryptorFor(ctx, encryptedDataKey)  // existing data key
dec, err := vault.Decryptor(ctx, encryptedDataKey)
```

`Open` resolves the backend from the environment, exactly as the knowledge store
already did: local gets the compiled-in dev backend, everything else gets AWS
KMS and requires `KMS_KEY_ARN`. Both produce the same at-rest shape, so no
caller branches on which one it got. A vault that cannot be opened stops the
server at startup instead of degrading to plaintext.

One vault is built in `main` and passed down. `knowledgestore.KMSBackend`,
`loadConfiguredKMSClient`, `loadKMSClient`, `recheckKMSClient`, and the
deployer's lazy `kmsClient` helper are gone, along with every inline
`kms.NewFromConfig`. Functions that took `(kmsKeyARN string, kmsClient
envelope.KMSClient)` now take a `*envelope.Vault`, which is what removes the
"which of these two arguments decides the storage format" question from
`EnsureDeploymentKey`, `EnsureDevKey`, `EnsureJudgeKey`, and the knowledge-store
credential helpers.

`Encrypt` on a nil encryptor is now an error. The one caller that stores values
in the clear on purpose, `deployment_build_env` for non-secret rows, already
branches on `IsSecret` and writes them itself. `Decrypt` still passes an
empty-nonce value through, because that is how those rows read back.

## Migration

None. No schema changes, no data changes, and both backends produce the shape
already in the tables.

`KMS_KEY_ARN` becomes required outside local mode. Preview and prod already set
it. An environment that ran without it was silently storing secrets in the
clear; it now fails to start.
