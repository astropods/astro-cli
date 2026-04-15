## Summary

Config values with leading or trailing whitespace (commonly introduced by copy-paste) were passed through to the API unchanged, causing "invalid token" errors that were difficult to debug. The fix trims whitespace at every point where config values enter the system — both CLI and UI.

## Design

**CLI:** Three layers are trimmed so all entry paths are covered regardless of how a value reaches storage:
- `ast configure set KEY VALUE` — the value argument is trimmed before use
- Interactive `ast configure` form — values are trimmed when collected from the TUI
- Config storage layer (`MergeProjectVars`) — trims before writing, catching any path not covered above
- `ASTRO_ACCESS_TOKEN` env var — trimmed when first read so a trailing newline from shell expansion doesn't break auth

**UI:** Text and secret/password input fields trim on `onChange`. Long-text, array, and object textareas are left as-is since their content may have intentional internal structure. The `.env` import path was already trimming correctly.

## Migration

No action required.
