## Summary

Adds `@@` as a self-referencing shorthand for `--var` on `blueprint deploy` and `agent redeploy`. When a secret name matches the variable key exactly, `KEY=@@` is equivalent to `KEY=@KEY`, reducing repetition when deploying agents that use conventionally named secrets.

## Design

`parseDeployVars` in the CLI gains one new case: when the value is exactly `@@`, the variable is resolved as a vault reference using the key name itself. The existing escape mechanism is generalised: `\@` as a prefix strips the backslash and treats the rest as a literal value, so `\@@` produces the literal string `@@`.

```bash
# Before
ast blueprint deploy my-agent --var ANTHROPIC_API_KEY=@ANTHROPIC_API_KEY

# After
ast blueprint deploy my-agent --var ANTHROPIC_API_KEY=@@

# Literal @@ (escaped)
ast blueprint deploy my-agent --var WEBHOOK_URL=\@@
```

The change is purely in CLI-side parsing — no server or spec changes required.

## Migration

No action required. Existing `KEY=@SECRET_NAME` syntax is unchanged.
