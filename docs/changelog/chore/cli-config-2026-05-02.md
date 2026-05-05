## Summary

Consolidates all CLI build-time configuration into a single source of truth, removes incidental complexity in config initialization, and introduces a `BuildType` enum so build-specific behavior is derived from the binary name rather than the version string.

## Design

**All nine ldflags now declared in `internal/buildinfo`** (`BinaryName`, `Version`, `Commit`, `DownloadBaseURL`, `WorkOSClientID`, `DefaultServerURL`, `DefaultRegistryURL`, `FleetServerURL`, `AmplitudeAPIKey`). Previously they were scattered across `cmd/root.go`, `internal/auth/config.go`, and `internal/telemetry/telemetry.go`.

**`BuildType` enum** (`prod`, `preview`, `dev`) is derived at init time from `BinaryName` and is the single authoritative signal for build-specific behavior. The default `BinaryName` is `"ast-dev"` so untagged `go build .` binaries are dev builds; production requires an explicit `-X ...BinaryName=ast` ldflag.

```go
switch BinaryName {
case "ast":         BuildType = BuildTypeProd
case "ast-preview": BuildType = BuildTypePreview
default:            BuildType = BuildTypeDev
}
```

All behavioral gates that previously compared `Version` to `"dev"` (dev command visibility, upgrade guard, update-check no-op) now compare `BuildType == BuildTypeDev`. Theme preview detection uses `BuildType == BuildTypePreview`. This prevents a mismatch where a production-named binary built without ldflags would expose dev-only commands due to the default `Version="dev"`.

**`AppDirName`** (`"." + BinaryName`) is computed once in `buildinfo.init()` and used everywhere a binary-scoped directory name is needed (home config dir, project-local state dir), replacing repeated `"."+binaryName` concatenations.

**`ConfigDir`** simplified from a switch statement with hardcoded binary names to `"." + binaryName`, and now guards against an empty name.

**`ast push` blueprint URL** now uses `buildinfo.DefaultServerURL` (the compiled-in platform URL) instead of `effectiveServerURL`.

## Migration

No user action required. Internal build scripts that set ldflags must update both the package path and variable name for any flags previously in `cmd.*`, `internal/auth.*`, or `internal/telemetry.*` — all are now `internal/buildinfo.*`. The flag values (version strings, URLs, keys) are unchanged. The release workflows in this PR are already updated.
