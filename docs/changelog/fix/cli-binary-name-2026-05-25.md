# Error messages reference actual binary name

## Summary

Eleven user-facing error and warning strings hardcoded the literal command name `ast` even when the binary was invoked as `ast-dev`, `ast-preview`, or any other build variant. Hitting one of those messages told the user to "Run 'ast login'" when they actually had to run `ast-dev login`. This change routes every such message through the binary-name resolved at runtime so the suggested command always matches the binary the user invoked.

## Design

The binary name comes from `buildinfo.BinaryName`, which is set at link time. Most call sites in `cmd/` already use it for similar messages — the eleven offenders were just literal strings that had been overlooked.

Three patterns are used depending on package layering:

- **`cmd/` package** — directly imports `buildinfo` and formats with `buildinfo.BinaryName`. Applied at `cmd/account.go`, `cmd/secrets.go` (two sites), and `cmd/messages.go`.

- **`internal/auth/`** — `Storage` and `TokenManager` already carry the binary name in their constructor (`NewStorage(binaryName)`, `NewTokenManager(binaryName)`) so the error paths use the existing receiver field (`s.binaryName`, `m.storage.binaryName`). No new parameters, no new imports.

- **`internal/compose/`** — added a direct `buildinfo` import (no cycle: `buildinfo` only depends on `fmt`). One Slack-adapter warning now uses `buildinfo.BinaryName`.

`cmd/messages.go`'s `errNoSpecFile` was a package-level `var` evaluated at init, so it captured `buildinfo.BinaryName` before the link-time value was meaningful for tests / multi-binary builds. It is now a function `errNoSpecFile()` returning a freshly formatted error at call time; the two callers (`cmd/repair.go`, `cmd/root.go`) were updated. No callers compared the value with `errors.Is`, so the conversion is safe.

## Migration

None. The messages themselves are unchanged in wording — only the command name they suggest is now correct for the binary the user invoked.
