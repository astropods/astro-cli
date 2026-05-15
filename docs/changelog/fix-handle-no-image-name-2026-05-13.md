## Summary

Blueprint builds could succeed with a spec that had no `image` and no `build` block on a component. The build pipeline would run zero jobs, transform nothing, and register the spec unchanged — then the agent would crash at deployment time because `agent.image` was missing. Additionally, structural spec errors (missing image/build, unsupported provider, invalid trigger type) were previously only collected as warnings at registration time, never causing a hard failure.

## Design

Structural validation is now centralized in `ValidateSpec` on the `Validator` type and called from both registration paths with consistent error splitting: structural errors (missing image/build, bad provider, invalid trigger type) cause a hard fail; deploy-time values (credentials, schedule expressions) are not known at registration and are stored as warnings.

Both paths now fail hard on structural errors:

- **GitHub build pipeline** — a new `ValidateSpec()` pipeline step runs after `FetchSpec()` and returns a `PermanentError` (no retry) listing all failing fields.
- **CLI push handler** — calls `ValidateSpec` before registration and returns `400` if any structural errors are found. Previously all errors from `ValidateSpec` were treated as warnings regardless of type.

Also fixed a pre-existing bug where integration providers (`github`, `gitlab`) were looked up under registry section `"tools"` instead of `"integrations"`, causing them to always fail provider validation with "unsupported provider" and generate spurious warnings in the database.

Build log errors are now surfaced in the build logs UI. The `BuildLogViewer` component accepts a unified `error` string; the dialog computes it from `build.error` (spec validation failure) or a pod-cleanup fallback, so validation failures are visible without inspecting the database.

## Migration

No action required. Specs that were previously accepted without an image or with other structural errors will now be rejected at push/build time with a clear error message.
