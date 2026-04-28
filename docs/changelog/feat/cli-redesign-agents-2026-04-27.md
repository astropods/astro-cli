## Summary

Adds `ast blueprint deploy` and `ast agent redeploy` for deploying blueprints and re-deploying running agents from the CLI.

## Design

### blueprint deploy

```
ast blueprint deploy <name> [flags]
ast deploy <name> [flags]          # top-level alias
```

Fetches the deployment template for a blueprint, merges user-supplied inputs, and submits the deployment. Flags:

| Flag | Description |
|---|---|
| `--name` | Display name for the new deployment |
| `--adapter web\|insecure-web\|slack` | Adapter to enable (repeatable); defaults to `web` |
| `--var KEY=VALUE` | Inline variable; `KEY=@SECRET_NAME` references an account secret; `KEY=@` self-references using the key as the secret name; `KEY=\@VALUE` escapes a literal `@` |
| `--vars-file` | Load variables from a `.env` file |
| `--build` | Pin to a specific build ID |
| `--dry-run` | Validate inputs without deploying |
| `--json` | Print JSON output on success |

### agent redeploy

```
ast agent redeploy <name> [flags]
```

Re-deploys an existing running agent in-place using its current deployment ID. Accepts the same `--adapter`, `--var`, `--vars-file`, `--build`, `--dry-run`, and `--json` flags as `blueprint deploy`. If `--adapter` is omitted, the adapter defaults to `web` — pass `--adapter` explicitly to preserve or change a non-web setup.

### Shared helpers

`registerDeployCommonFlags` and `parseDeployVarsFromCmd` are extracted so `deploy` and `redeploy` share flag registration and variable parsing without duplication.

## Migration

No action required. These are new commands with no changes to existing behaviour.
