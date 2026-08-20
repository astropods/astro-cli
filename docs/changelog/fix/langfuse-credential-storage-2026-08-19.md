# One way to store a Langfuse credential

## Summary

The per-account Langfuse secret key had two storage shapes and three ways of
reading it, and no combination of them worked for every account. Provisioning
encrypted the key when it was handed a KMS client and stored it in the clear
when it wasn't. Readers split the same way: most passed the stored column
straight to Langfuse as a basic-auth password, while the judge worker and
dataset predictions ran it through envelope decryption. Each half worked for the
accounts the other half broke.

The stored data settled the question. Of 112 rows, 111 hold the key exactly as
it was issued, and 1 holds real ciphertext. Encryption has therefore never been
the operating state, only an intermittent one, and every account that works
today works because a reader ignored it. This change keeps the shape the data is
already in: the key is stored as issued, and there is one way to read it.

## Design

`account_langfuse.langfuse_secret_key` holds the key as Langfuse issued it. The
database volume is KMS-encrypted at rest, and the key is the password half of
Langfuse basic auth, so every consumer needs its exact bytes.

That collapses the code to one path in each direction:

- `Store.Get` returns credentials ready to use. `GetDecrypted`,
  `decryptSecretKey`, and `ErrCredentialsDecrypt` are gone, along with the
  key-material fields on `AccountLangfuse`.
- `EnsureProject(ctx, store, accountID, accountName)` no longer takes a KMS key
  ARN or a client, so a caller cannot select a storage shape by what it passes.
  This is what removes the original defect: the deploy path constructed its
  deployer without a KMS client and silently took the plaintext branch, while
  the ingest-key path passed one and took the encrypted branch.
- astro-otel reads the two key columns and nothing else. Its `internal/envelope`
  package existed only to decrypt this one credential and is deleted with it.

## Migration

One account holds a genuinely encrypted key, and nothing decrypts it any more.
Delete its row and re-provision:

```sql
DELETE FROM account_langfuse
WHERE langfuse_secret_key NOT LIKE 'sk-lf-%';
```

Then run **Recover Langfuse** for that account from the admin console.
`EnsureProject` matches the existing Langfuse project by name, so the account
keeps its trace history and receives a fresh key. Deploying an agent or creating
an ingest key does the same thing. Until one of them runs, that account resolves
no credentials, so its ingest is dropped and its observability pages are empty.

`encrypted_data_key` and `nonce` are now unread. Drop them in the next schema
apply, once both services are running this code:

```sql
ALTER TABLE account_langfuse DROP COLUMN encrypted_data_key, DROP COLUMN nonce;
```

astro-otel's IRSA role no longer needs `kms:Decrypt` on the deployment-secrets
key. astro-server still needs `GenerateDataKey` and `Decrypt` there for
deployment secrets, knowledge-store credentials, and AI Gateway keys.
