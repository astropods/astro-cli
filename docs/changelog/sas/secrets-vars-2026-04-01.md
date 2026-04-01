# Secrets & Variables Vault

## Summary

Adds account-scoped secrets and variables management to Astro. Users can store credentials and configuration values once and reference them across agent deployments — instead of re-entering values per deployment or embedding secrets in specs.

## Design

### Backend (already shipped on this branch)

The server stores per-account variables in `account_variables`. Secrets are AES-256-GCM encrypted at rest using a per-account data key managed by AWS KMS (envelope encryption). The schema:

- `account_variables(account_id, name, value, secret, nonce, description, created_at, updated_at)`
- `account_encryption_keys(account_id, encrypted_data_key, kms_key_arn)`

Four REST endpoints under `/api/v1/accounts/:account/variables`:
- `GET` — list variables (values omitted for secrets)
- `POST` — create a variable
- `PUT /:name` — update value/description/secret flag
- `DELETE /:name` — delete

Variable references in deployment specs use a `ref` field:

```yaml
variables:
  API_KEY:
    ref: MY_API_KEY   # resolved server-side, value never leaves the server
```

### Frontend (this PR)

**Settings page** at `/settings/secrets` — full CRUD for the vault:
- Create secrets (encrypted) or variables (plaintext)
- Update variable value/description; overwrite secret value
- Delete with confirmation
- Bulk import from `.env` file content

**VaultPicker** — a key icon button on text/secret fields in the deploy form. Opens a searchable popover of all vault entries. Selecting one inserts a token like `{{secrets.MY_API_KEY}}` or `{{vars.BASE_URL}}` into the field, displayed as a teal chip.

When the deployment spec is submitted, tokens are converted to `ref: "NAME"` before the API call. The backend resolves them server-side before provisioning the pod.

```
User selects vault entry  →  field shows {{secrets.MY_KEY}}
                          ↓
                     buildSpec()
                          ↓
  variables.MY_KEY = { ref: "MY_KEY", ... }   (sent to API)
                          ↓
              server resolves + decrypts
                          ↓
              pod receives plaintext env var
```

## Migration

No migration needed. The vault is empty by default; existing deployments with plain values continue to work unchanged. Variable names must match `^[A-Z][A-Z0-9_]*$`.
