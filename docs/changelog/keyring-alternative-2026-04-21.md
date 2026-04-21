## Summary

The `ASTRO_NO_KEYRING` environment variable escape hatch has been removed. It was introduced alongside the `testing.Testing()` guard as a secondary mechanism for suppressing macOS Keychain prompts. Two guards now cover all cases without needing an explicit env var.

## Design

`isKeyringAvailable()` in `internal/auth/storage.go` skips the keyring probe under two conditions:

1. **`testing.Testing()`** — any `go test` binary returns `false` immediately; the unsigned binary never touches the macOS Keychain dialog.

2. **`isHomeTempDir()`** — when the process's `HOME` directory lives inside the system temp directory (e.g. e2e tests that set `HOME=t.TempDir()`), macOS cannot locate `~/Library/Keychains/login.keychain-db` and would show a "Keychain Not Found" dialog. The new helper detects this by symlink-resolving both `$HOME` and `os.TempDir()` before comparing, which handles the macOS `/var → /private/var` alias correctly.

The previous `ASTRO_NO_KEYRING` path had significant maintenance overhead: a const, a helper function, and a dedicated provenance guard test that walked the entire CLI source tree on every run to enforce that the literal string only appeared in an allowlist of files.

All three are removed:
- `keyringForceDisabledEnv` const
- `keyringForceDisabled()` function and its call in `isKeyringAvailable()`
- `keyring_escape_hatch_provenance_test.go`

## Migration

No action required. If you were setting `ASTRO_NO_KEYRING=1` in a local script or CI environment, the variable can be removed — it is no longer read.
