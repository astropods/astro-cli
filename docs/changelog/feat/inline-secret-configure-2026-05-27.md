# Inline secret configure / redeploy

## Summary

Configure and redeploy flows dropped inline deployment secrets: the form showed empty password fields even when a value was stored, and redeploys that omitted unchanged secrets could fail validation or lose credentials. Vault references (`{{secrets.NAME}}`) already round-tripped; this change covers secrets entered directly at deploy time.

## Design

**Template API metadata.** `Variable.configured` is returned on `TemplateResponse.variables` when an inline secret exists in `deployment_build_env` (no vault `ref`). The value is never included on configure prefill (`finalize=false`). The flag is stripped from `deployment/v1` specs before persistence, same as other template-only fields.

**Server preserve on finalize.** When `finalize=true` and the client omits a stored inline secret, the handler decrypts from `deployment_build_env` (KMS or plaintext dev path) and injects it into `req.Variables` before `ShapeTemplate`. The promoted `variables` schema map is still scrubbed (`configured: true`, empty `value`) so the UI never receives plaintext; the shaped `template` retains values for signing, matching the existing flow when users type secrets in the form.

**Client.** `SecretField` shows a read-only password input with a `••••••••` placeholder when `configured` is set; clicking the field switches to edit mode. `isVariableFilled` treats configured secrets as satisfied so deploy validation passes without re-entry.

## Migration

No action required. Prefer vault references for secrets that must be visible by name in the UI; inline secrets now show a configured state instead of appearing unset.
