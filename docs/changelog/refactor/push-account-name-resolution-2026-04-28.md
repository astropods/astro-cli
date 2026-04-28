## Summary

Fixes two bugs in `ast push` / `ast blueprint push` name and account resolution, lifts that logic out of the low-level `runPush` into its caller, adds a `-y/--yes` flag to skip the visibility confirmation prompt, and improves two `ast blueprint deploy` UX rough edges: the `-n` shorthand now maps to `--name` instead of `--dry-run`, and a 409 conflict returns an actionable error instead of a raw JSON blob.

## Design

### Bugs fixed

**Org-scoped spec name used as Docker image tag.** When the spec declares `name: "@org/hello-world"` and no name argument is given, the full org-scoped string was forwarded to Docker as the image tag, producing `invalid reference format`. The fix strips the org prefix with `ParseAgentName` before passing the name downstream.

**Account mismatch false-positive with explicit name arg.** When running `ast push foo` against a spec named `@org1/hello-world` while logged in as `org1`, the old code re-parsed the arg to extract an account, then compared a blank extracted account against the logged-in account. The fix removes account parsing from `runPush` entirely — the account always comes from the login token.

### Architecture

Name and account resolution now lives exclusively in `runBlueprintPush`:

1. Parse `specAccount` and `agentName` from the spec via `ParseAgentName`.
2. If `specAccount` is set and differs from `at.Account`, return an account mismatch error (or print an override warning if `--allow-account-override` is set).
3. If an arg is provided, use it as `agentName` (warn if it differs from the spec name).
4. Pass the resolved bare `agentName` to `runPush` via `pushConfig`.

`runPush` no longer needs `allowAccountOverride` in `pushConfig` and no longer imports `utils`. It trusts the name it receives.

### Tests

A single table-driven `TestRunBlueprintPush` covers all eight combinations of spec format, account match/mismatch, name arg, and override flag:

| Case | Spec | Arg | Override | Expected |
|------|------|-----|----------|----------|
| bare spec, no arg | `my-agent` | — | — | registers `my-agent` |
| org-scoped, matching account, no arg | `@alice/my-agent` | — | — | registers `my-agent`, no warning |
| org-scoped, mismatch, no arg | `@acme-corp/my-agent` | — | — | account mismatch error |
| org-scoped, mismatch, no arg + override | `@acme-corp/my-agent` | — | ✓ | account warning only |
| arg matches spec name | `my-agent` | `my-agent` | — | no override warning |
| arg differs from spec name | `my-agent` | `new-name` | — | name override warning |
| arg + mismatch, no override | `@acme-corp/my-agent` | `my-agent` | — | account mismatch error |
| arg differs + mismatch + override | `@acme-corp/my-agent` | `fooo` | ✓ | both warnings, `fooo` registered |

### `-y/--yes` skips visibility confirmation

Pushing with `-V public` (or changing from public to private) triggers an interactive confirmation prompt. Pass `-y` or `--yes` to skip it for non-interactive use:

```
ast push -V public --yes
ast blueprint push my-agent -V public -y
```

### `blueprint deploy -n` shorthand

`-n` was the shorthand for `--dry-run` but users naturally reach for it as short for `--name`. The shorthand is now on `--name`; `--dry-run` has no shorthand.

```
ast blueprint deploy weather-poet -n my-deployment   # sets display name
```

### `blueprint deploy` 409 conflict error

A name conflict on the deploy POST used to surface as a raw JSON error. It now returns an actionable message:

```
deployment name "abc1" is already in use — choose a different name:
  ast deploy <blueprint> --name <new-name>
```

## Migration

| Old | New |
|-----|-----|
| `ast blueprint deploy <bp> -n` | `ast blueprint deploy <bp>` (`-n` now sets `--name`, not `--dry-run`) |

The `--allow-account-override` flag and all push warnings are unchanged.
