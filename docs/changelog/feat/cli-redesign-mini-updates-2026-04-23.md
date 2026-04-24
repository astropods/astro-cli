# CLI: secret and blueprint quality-of-life improvements

## Summary

A set of focused improvements to the `ast secret` and `ast blueprint` command families: name validation on create, description support for secrets, agent card rendering in `bp get`, and deployment variable inspection via `bp get --template`.

## Design

### Name validation

`secret create` and `blueprint create` now validate names before making any API call. Secret names must be at least 4 characters and contain only letters, digits, or underscores. Blueprint names follow the same rule but also allow hyphens. Rejection is immediate with a clear error message.

### Secret descriptions

`secret create` and `secret update` both accept `--description` / `-d`. The value is passed to the API and rendered by `secret get` (which already returned the field). In interactive mode both commands include a description prompt, pre-filled from the flag if provided; an empty value after trimming is silently ignored. `secret get` renders Description last. The `secrets` command is also accessible as `secret` (alias).

### Blueprint agent card

`bp get` gains a `--card` flag that fetches and renders the agent's description as markdown via glamour (auto dark/light mode). The source follows the same priority the web client uses: latest version's `agent_card.body` → `draft_card.body` → latest version's `readme`. The card is shown after the version list; when no versions exist the draft card is shown alongside the first-push prompt.

### Deployment variables

`bp get --template` fetches the blueprint's deployment template and renders the variable list below the version section. Variables are sorted required-first then optional, both groups alphabetical. Each row shows the variable name, description (truncated at 60 characters), and notes — `(secret)` for sensitive values, `(optional)` or `(default: …)` for optional ones. The `--template` flag can be combined with `--card`.

## Migration

No changes required. All new flags are opt-in.
