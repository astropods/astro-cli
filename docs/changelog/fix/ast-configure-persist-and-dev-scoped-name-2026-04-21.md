# `ast configure` persistence and `ast dev` scoped-name cleanup

## Summary

Two reported regressions in the CLI:

1. `ast configure` stopped persisting values across runs after a recent upgrade. Re-running the interactive form silently overwrote previously stored secrets with the empty strings the form submits for untouched fields, and in some cases `ast configure` could not see values written by `ast create` at all.
2. `ast dev` printed `WARN[0001] Warning: No resource found to remove for project "@org/name"` for specs with scoped names. The tear-down/logs/health paths were calling Docker Compose with the raw spec name (`@org/name`), while `BuildProject` had sanitized it to `name`, so every cleanup call targeted a project that didn't exist.

Both fixes ship with unit and integration coverage so the regressions can't come back silently.

## Design

### `ast configure` persistence

The store layer (`internal/config/storage.go`) now has a clear two-verb contract:

- `MergeProjectVars` upserts only non-empty values (after trimming). Blank submissions from the interactive form are treated as "no change" and preserve whatever was already stored. This is what keeps pre-filled secrets around across repeated `configure` runs.
- `UnsetProjectVars` is the explicit removal path. `ast configure unset <KEY> [KEY...]` is a new subcommand; `ast configure set KEY ""` is routed through the same path so the previous "empty value clears the key" behavior of `set` is preserved as an observable contract without leaking empties into `MergeProjectVars`.

The configure banner is updated to match the new semantics — it reports saved, preserved (blank but stored value survived), and skipped (blank with nothing stored) separately, and nudges users toward `unset` when they actually want to clear a secret.

The second half of bug 1 was a path-key mismatch on macOS: `ast create` stored the project key using `filepath.Abs(targetDir)` (which does not resolve symlinks), while `ast configure` and `ast dev` looked it up via `os.Getwd()` (which does). On macOS this means `/var/folders/...` vs. `/private/var/folders/...`, so the two halves of the workflow wrote to and read from different keys. `LoadProjectConfigs` now canonicalizes every project key via `filepath.EvalSymlinks` when the file is read, migrating legacy un-canonicalized keys in place. Collisions between a raw and canonical key are merged with canonical values winning for keys that are present in both, so live configs never get silently overwritten by stale ones.

### `ast dev` scoped-name cleanup

Compose project-name construction is now centralized in `internal/compose`:

- `ProjectName(spec)` — sole source of truth for the compose project name derived from an `AstroSpec`.
- `ProjectNameFromSpecName(raw)` — same logic against a raw string, for callers that only have the bytes from a legacy `.running` state file.

`BuildProject` uses `ProjectName` for both the compose project and network. `dev.go` now routes every Down, Logs, health-check, and `.running` state read/write through these helpers, so the name used at Up time is identical to the name used everywhere else. `readDevProjectName` normalizes legacy scoped values in `.running` files so `ast dev logs` and `ast dev stop` continue to work against projects started by older CLIs.

### Testing

- Unit tests cover `MergeProjectVars` skip-empty semantics, the new `UnsetProjectVars` API, path canonicalization (create-then-configure round trip + legacy un-canonicalized key readability), compose project-name derivation, and `.running` state normalization.
- An integration test boots a real `nginx:alpine` container through the compose Go SDK with a scoped spec name and asserts that (a) `Down` with the raw scoped name silently no-ops — this is the reproduction of the user's warning — and (b) `Down` with the sanitized project name actually tears the containers down.
- A second integration test drives the real built CLI as a subprocess against a temporary HOME and exercises `configure set`, `set KEY ""` → unset routing, explicit `unset`, and symlinked-cwd path consistency. It also asserts that `configure set` never echoes secret values to stdout/stderr.
- The integration suite runs under the `integration` build tag via a new `astro-cli:e2e` Moon task (`runInCI: false`, because it requires a local Docker daemon). A new `astro-cli:test` target covers the always-on unit suite.

### Keyring probe under tests

`auth.NewStorage` eagerly probes the Keychain on every construction via a `keyring.Set` / `keyring.Delete` round trip. On macOS this triggers a permission prompt any time an unsigned binary calls it, including `go test` binaries, which blocked automated runs as soon as a test exercised a code path that constructed a storage. Two gates now short-circuit the probe before it hits the Keychain:

1. `testing.Testing()` — any binary produced by `go test` skips the probe unconditionally. Tests never want to read or write real Keychain entries, and the standard-library signal means this works for every package's test suite without per-package `TestMain` plumbing. The production `ast` binary built by `go build` is unaffected.
2. `ASTRO_NO_KEYRING=1` — escape hatch for the integration tests (and CI/ops) that drive the real built CLI as a subprocess, where `testing.Testing()` is false because the subprocess is a `go build` artifact. Only the exact value `"1"` disables the probe; any other value falls through to the live probe so a shell-quoting accident (`ASTRO_NO_KEYRING=true`) can't silently downgrade credential storage to a plaintext file in production. A provenance test walks `apps/astro-cli` and fails if the env var literal appears outside an allowlist, so new read or write sites for the hatch require a deliberate allowlist update.

### Comment-style enforcement

The CLI house style is `//` for both single-line and multi-line comments. `gofmt`, `gofumpt`, `go vet`, `staticcheck`, and the linters currently enabled in `.golangci.yaml` all preserve `/* … */` without complaint, so the rule had no enforcement. A new `internal/lintcheck` package ships a test that AST-walks `apps/astro-cli`, parses every `.go` file with comments, and fails with file:line offenders if any `/* … */` comment is found. Running under the always-on unit suite makes this CI-visible without adding a new tool dependency.

## Migration

None. Existing specs and stored configs keep working. Users with project configs written by older CLIs will have their entries transparently canonicalized the next time `ast configure` or `ast dev` runs. Scoped-name specs (`@org/name`) that previously produced the "No resource found to remove" warning will no longer emit it. Users who relied on `ast configure set KEY ""` to clear a value get the same observable result; the new preferred form is `ast configure unset KEY`.
