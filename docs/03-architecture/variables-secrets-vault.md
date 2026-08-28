# Variables and secrets vault

**Status:** Authoritative — describes the shipped system
**Last verified:** 2026-08-26

How account-scoped variables and secrets are stored, encrypted, and
referenced from a deployment. Covers the backend store
(`internal/accountvars`), the shared envelope-encryption primitive
(`internal/envelope`), the CRUD handlers (`handlers/account_secrets.go`),
and the frontend vault UI (settings pages, `VaultPicker`, `VariableField`).

## Variables vs. secrets: one table, one flag

A "variable" and a "secret" are the same row shape in `account_variables`.
The only difference is the `secret` boolean:

- `secret = false`: the value is stored as plaintext and returned as
  plaintext by every read endpoint.
- `secret = true`: the value is AES-256-GCM ciphertext (base64-encoded in
  the `value` column), with a 12-byte nonce in a separate `nonce` column.
  No read endpoint ever returns the value; `List` and `Get` omit it.

There's no separate "secrets" mechanism or table. `internal/accountvars`
(`apps/astro-server/internal/accountvars/store.go`) manages two tables:

- `account_variables`: one row per `(account_id, name)`, holding
  `value`, `secret`, `nonce`, `description`, timestamps.
- `account_encryption_keys`: one row per `account_id`, holding the
  account's KMS-wrapped data key (`encrypted_data_key`, `kms_key_arn`).
  Created lazily on the first secret write for that account — an account
  that only ever stores plaintext variables never gets a row here.

Changing a variable's `secret` flag without submitting a new value is
rejected by the API (`UpdateAccountVariable` in
`handlers/account_secrets.go`): flipping the flag would require either
encrypting a value the server no longer has in plaintext, or decrypting a
value the caller didn't ask to see, so the handler requires a fresh value
whenever the flag changes.

## Encryption model

### Envelope encryption primitive

`internal/envelope` (`apps/astro-server/internal/envelope/envelope.go`,
`vault.go`) implements AWS KMS envelope encryption, generically, for any
caller that needs to store secret values at rest:

- `Vault.Encryptor(ctx)` calls KMS `GenerateDataKey` to mint a fresh
  256-bit AES data key, wrapped by the deployment's KMS customer master
  key (CMK). The plaintext data key encrypts values locally with
  AES-256-GCM; only the wrapped (`EncryptedDataKey`) form is persisted.
  The plaintext key is zeroed after the AES-GCM cipher is constructed.
- `Vault.EncryptorFor(ctx, encryptedDataKey)` calls KMS `Decrypt` on an
  existing wrapped data key and rebuilds an `Encryptor` from it, so new
  values can be added under an entity's existing key instead of
  generating (and paying for) a new one.
- `Vault.Decryptor(ctx, encryptedDataKey)` calls KMS `Decrypt` and returns
  a `Decryptor` for reading existing ciphertext.
- Every value gets its own random 12-byte nonce (`Encrypt` returns
  `ciphertext, nonce`); nothing reuses a nonce under the same data key.
- `Decrypt(ciphertext, nonce)` treats an empty nonce as a plaintext
  passthrough. This lets every caller run non-secret and secret rows
  through the same decrypt call without an "is this row encrypted?"
  branch — see the doc comment on `Decryptor.Decrypt` in `envelope.go`.

There is no KMS-off fallback left in the encryption path itself (a nil
`Decryptor` used against a non-empty nonce is now a hard error, not a
silent plaintext read — see the 2026 refactor
`9868aebeb refactor(envelope): one vault for encryption, no plaintext
fallback`). The remaining "no KMS" behavior — storing a variable in
plaintext — is a deliberate per-value choice (`secret = false`), not a
degraded mode of the encryption code.

### Key hierarchy

```
KMS CMK (cfg.Deployment.KMSKeyARN, or local-dev master key)
  └─ per-entity data key (KMS GenerateDataKey, wrapped, stored on the entity's row)
       └─ per-value AES-256-GCM ciphertext + nonce
```

The CMK itself is never used to encrypt a variable's value directly —
only to wrap/unwrap each entity's data key. Every entity type that stores
secrets generates and stores **its own** wrapped data key; there's no
single account-wide or server-wide data key shared across entities:

- `account_encryption_keys.encrypted_data_key` — one per account, used
  for that account's `account_variables` rows.
- `deployments`' own data key column — one per deployment, used for
  inline secrets and build-env values in `deployment_build_env`
  (`internal/deploymentstore/build_env.go`, `build_env_decrypt.go`,
  `inline_secret.go`).
- `knowledge_stores.encrypted_data_key` — one per connected knowledge
  store, used for its connection credentials
  (`internal/knowledgestore/credentials.go`).
- AI Gateway dev keys carry their own wrapped data key per issued key
  (`internal/aigateway/provisioner.go`, `encryptAPIKey`/`decryptAPIKey`),
  freshly generated per key rather than reused.

A single `*envelope.Vault` (`deps.Vault`, opened once in `main.go` via
`envelope.Open`) is threaded into all of these call sites. The Vault
itself holds only the KMS client and the CMK ARN — it has no state that's
specific to accounts, deployments, or any other entity, so it's safe to
share as one process-wide dependency.

### Local dev mode

When `cfg.Deployment.IsLocal()` is true (`ENVIRONMENT=local`),
`envelope.Open` swaps the real KMS client for `LocalKMSClient`
(`internal/envelope/localkms.go`), which emulates `GenerateDataKey` and
`Decrypt` against a compiled-in, publicly-visible AES-256 key
(`localDevMasterKey`). It produces the exact same on-disk shape
(ciphertext + nonce + wrapped data key) as production KMS, so no caller
branches on which backend is active. This key provides no confidentiality
and must never be reachable outside local-mode dev/test.

## Account-level vs. org-level scoping

There is no separate "org secrets" mechanism. `account_variables` is keyed
by `account_id`, and both personal accounts and organization accounts are
rows in the same `accounts` table (`acct.Type` is `"personal"` or
`"organization"`). The frontend's `OrgSecretsSettings.tsx` is a thin
wrapper around the same `VaultSettings` component used by
`SecretsSettings.tsx`, passing the org's slug as the account name instead
of the user's personal account name:

```tsx
export default function OrgSecretsSettings() {
  const { orgSlug = '' } = useParams()
  return <VaultSettings account={orgSlug} />
}
```

Authorization for both is the same middleware chain
(`main.go`, `accountVarsRead`/`accountVarsWrite` route groups):

- `middleware.ResolveAccount` resolves `:account` from the path.
- `middleware.RequireAccountPermission(accountStore, "deployments:read")` (read
  routes) or `"org:manage"` (write routes) enforces the permission.

The dedicated `variable:read` and `variable:write` slugs the vault used to
check are not org role permissions in WorkOS. The FGA model
(`scripts/workos-fga/model.json`) defines `variable:read` and `variable:manage`
as account permissions, but nothing enforces them yet. Both route groups borrow
an existing org role permission until FGA does, so vault read follows the same
gate as viewing a deployment. See
[`04-guides/workos-org-rbac-setup.md`](../04-guides/workos-org-rbac-setup.md)
and [`01-spec/private-by-default-fgac-rollout.md`](../01-spec/private-by-default-fgac-rollout.md).

A variable is not an FGA resource. The vault is one account-scoped keyspace,
so access is an account permission and no `authz` resource is registered when a
variable is created or deleted.

`RequireAccountPermission` (`internal/middleware/account.go`) branches on
account type:

- **Personal account:** the only check is membership (the creator is the
  sole member and implicitly has every permission).
- **Organization account:** the caller's session must be scoped to that
  WorkOS organization (`session.OrganizationID == acct.WorkOSOrganizationID`,
  otherwise a 403 telling the caller to switch org first), and the
  `deployments:read`/`org:manage` permission must be present on the JWT for
  the caller's role. Every role carries `deployments:read`, so owner, admin,
  and member all read the vault; only owner and admin carry `org:manage`, so
  writes stay with them.

The frontend mirrors this gate client-side (`VaultPicker`'s `canCreate`)
purely as a UX affordance — hiding the "+ New" button for a read-only org
member — not as the actual enforcement point; the server re-checks on
every write.

## API surface

Routes (`main.go`, tag `Variables`):

| Method | Path | Permission | Handler |
|---|---|---|---|
| GET | `/api/v1/accounts/:account/variables` | `deployments:read` | `ListAccountVariables` |
| GET | `/api/v1/accounts/:account/variables/:varName` | `deployments:read` | `GetAccountVariable` |
| POST | `/api/v1/accounts/:account/variables` | `org:manage` | `CreateAccountVariable` |
| PUT | `/api/v1/accounts/:account/variables/:varName` | `org:manage` | `UpdateAccountVariable` |
| DELETE | `/api/v1/accounts/:account/variables/:varName` | `org:manage` | `DeleteAccountVariable` |

`CreateAccountVariable` accepts a batch (`{"variables": [...]}`) and
returns a per-entry result (`created` or `error` with a message), so a
bulk import can partially succeed — one invalid name doesn't fail the
whole batch. It validates each name with `spec.IsValidVarName` (shared
with `astro-spec`) before touching the store.

An encryptor is built once per request (not once per variable) when any
entry in the batch is a secret: `getAccountEncryptor` looks up the
account's existing `account_encryption_keys` row and calls
`vault.EncryptorFor`, or — the first time the account writes a secret —
calls `vault.Encryptor` fresh and persists the new wrapped data key.

## Referencing a variable at deploy time

A deployment spec's `variables` map can either carry a literal `value` or
a `ref` pointing at an account variable by name. `resolveVarReferences`
(`handlers/account_secrets.go`, called from `handlers/deploy.go` before
`deployment.ValidateAndResolve`) resolves every `ref` against the
account's `account_variables` before the spec is validated:

1. Collects and deduplicates all referenced account-variable names.
2. Batch-fetches them with `Store.GetByNames`.
3. Rejects the request if any referenced name doesn't exist.
4. Rejects the request if an account variable's `secret` flag doesn't
   match the deployment field's `secret` flag — a secret account variable
   can only resolve a secret deployment field, and vice versa. This is
   checked before decryption, so a secret value can never end up copied
   into a plaintext deployment field and persisted as ordinary
   configuration.
5. Builds one shared `Decryptor` (only if at least one referenced
   variable is a secret) and decrypts each referenced value into the
   deployment spec, clearing `Ref` so downstream validation and the K8s
   manifest see only the resolved value.

The resolved refs map (spec key → account variable name) is returned so
the caller can persist it, which is what lets a redeploy remember which
field was backed by which vault entry.

### Related but distinct: inline secrets on a deployment

A deployment field can also hold a literal secret value typed directly
into the form (an "inline secret"), with no vault reference at all. That
value is encrypted under the *deployment's own* data key and stored in
`deployment_build_env`
(`internal/deploymentstore/build_env.go`), not in `account_variables`.
The deployment template's `Configured` flag
(`internal/deployment/deployment_spec.go`) marks a field that already has
an inline secret stored from a prior deploy, so a redeploy form can show
a masked placeholder without re-fetching or re-typing the value. This is
a deployment-scoped mechanism, covered by the deployment area's own doc
(`03-architecture/deployment-state-machine.md`) and code paths
(`internal/deploymentstore/**`, `internal/deployment/**`) — it shares the
`envelope` package with the vault described here, but not its storage or
its account scoping.

## Frontend

### Settings pages

`SecretsSettings.tsx` (personal account) and `OrgSecretsSettings.tsx` (org
account) both render `VaultSettings`, the shared list/create/edit/delete
UI, parameterized only by which account name to operate on. `VaultEntry`
(`src/lib/vault.ts`) is the display shape: `name`, `type`
(`'secret' | 'variable'`), `description`, `updatedAt`, and an optional
`value` (present only for plain variables).

- **Create** (`NewEntryDialog`): a multi-row form (up to 30 rows per
  submission) supporting manual entry, `.env`/JSON/text file upload, or
  paste. Each row has an independent `secret` toggle (defaults to secret)
  and optional description. Duplicate names, invalid names
  (`VARIABLE_NAME_PATTERN`), and empty values block submission
  client-side; the server re-validates and re-checks duplicates
  independently.
- **Edit a plain variable** (`EditVariableDialog`): value and description
  are both editable and shown in the clear, since a plain variable's
  value is never secret.
- **Edit a secret** (`OverwriteSecretDialog`): the value field is always
  blank on open (the current value is never fetched); leaving it blank
  and saving only changes the description. Entering a value replaces the
  secret permanently — there is no way to read back what was there
  before.
- **Delete**: irreversible; the confirmation dialog calls out that a
  secret's value can't be recovered.

### Bulk import

Two independent import paths exist, both built on the same
`parseVariables`/`parseEnvLines` parser (`src/components/deploy/parse-env.ts`),
accepting `.env`, `.json`, or `.txt` (max 256 KB):

- **Vault import** (`NewEntryDialog`'s "Import .env" menu): adds parsed
  `KEY=VALUE` pairs as new rows in the create-variable dialog. All
  imported rows default to `secret = true`. Import is available via file
  upload, drag-and-drop, or a paste dialog; up to 30 rows total, with a
  warning when a file's contents would exceed that.
  Note: `NewEntryDialog`'s standalone `ImportVariables.tsx` component
  (used elsewhere for deploy-form variable filling) reports a
  `matched`/`skipped` result instead of adding rows — it exists to
  auto-fill a deployment's declared variable fields from an uploaded
  file, not to create vault entries.
- **Deploy-form import**: the same parser fills a blueprint's declared
  variable fields directly (no vault entry is created); a key that
  doesn't match any declared field is reported as "skipped."

### Referencing a vault entry from a deploy form: `VaultPicker`

`VaultPicker` (`src/components/deploy/VaultPicker.tsx`) is the popover
attached to each variable/secret field on a deploy form. Selecting an
entry writes a token into the field — `{{vars.NAME}}` for a plain
variable, `{{secrets.NAME}}` for a secret
(`buildVaultToken`/`parseVaultToken`) — which is what the backend's
`resolveVarReferences` expects to find in a submitted spec's `ref`.
`VaultPicker` filters its list to only the entries whose `secret` flag
matches the field's `expectedSecret`, mirroring the backend's
type-compatibility check.

`VaultPicker` also embeds the same `NewEntryDialog`, seeded from the
field that opened it (`newVarName`/`newVarValue`/`newVarSecret`), so a
user can create a missing vault entry without leaving the deploy form. On
success it either fills the field that opened the picker (single entry
created) or, for a multi-entry create, fans the new tokens out to every
matching field on the form via `bulkSetVariables` — the same mapping path
used by `.env` import.

For a deploy targeting an org account the caller isn't currently scoped
into, `VaultPicker` transparently calls `switchOrg` before it will show
entries or allow creation, coalescing concurrent switch calls across
multiple field pickers on the same page (`inflightScopeSwitches`) because
WorkOS refresh-token rotation can't tolerate parallel org-switch calls
for the same target org.

### Auto-fill on a fresh deploy form

`VariableField`'s `useVaultAutoFill` hook offers each field exactly one
auto-fill opportunity, once vault entries have loaded: if the field is
still empty and a vault entry's name matches the field's key (exact
match first, case-insensitive match second), it fills the field with
that entry's token and marks it as auto-filled (an "Auto-filled" badge,
click-to-reopen-the-picker if there was more than one candidate). The
opportunity is consumed once evaluated, whether or not a fill happened,
so clearing an auto-filled value never re-triggers it, and any explicit
user edit (including re-selecting the same entry) clears the
auto-fill provenance marker.

Auto-fill is enabled **only** for a fresh deploy with no prior values
(`vaultAutoFillEnabled: !opts?.deploymentId && initialValues !== null` in
`useDeployForm.ts`). A redeploy or the configure page preserves the
loaded deployment's values exactly — it never runs auto-fill matching,
so re-opening an existing deployment doesn't silently swap a
manually-typed value for a same-named vault entry.

## Real code issues and inconsistencies found

- `internal/accountvars` has no test file of its own (`store.go` has no
  `store_test.go`). Its SQL is exercised only indirectly, through
  `handlers/account_secrets_test.go`'s `sqlmock`-backed handler tests —
  real coverage of the query shapes, but no isolated unit test of the
  store package.
- `UpdateAccountVariable`'s "can't change `secret` without a new value"
  rule is enforced only in the handler, with a comment calling it out as
  ad hoc ("this is tricky so we require a new value"); there's no
  store-level invariant preventing an inconsistent `(secret, nonce)`
  pair from being written by a future caller that skips the handler.
- The frontend's `canCreate` gate in `VaultPicker` is explicitly a UX
  mirror of the server's write check, not itself
  authoritative (correctly documented in a comment) — worth knowing if
  auditing for authorization bugs, since the real enforcement is
  entirely server-side in `RequireAccountPermission`.

## Verify

Backend tests are `sqlmock`-based (no live Postgres or KMS required):

```
cd apps/astro-server
go test ./internal/envelope/... ./handlers/... -run 'AccountVariable|ResolveVarReferences|ValidVarName|SecretStorageIsAlwaysEncrypted'
```

`internal/accountvars` itself has no test files — `go test
./internal/accountvars/...` reports "no test files" rather than
verifying anything; the store's SQL is covered indirectly via the
handler tests above.

Frontend:

```
cd apps/astro-client
bun x vitest run src/lib/vault.test.ts src/pages/settings/SecretsSettings.test.tsx \
  src/components/settings/secrets \
  src/components/deploy/vault-picker.test.ts src/components/deploy/vault-picker.role.test.tsx \
  src/components/deploy/vault-picker.scope.test.tsx src/components/deploy/vault-picker.prefill.test.tsx \
  src/components/deploy/VariableField.test.ts src/components/deploy/VariableField.configured.test.tsx \
  src/components/deploy/VariableField.autofill.test.tsx
```
