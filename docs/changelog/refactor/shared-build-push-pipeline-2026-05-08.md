# Shared build/push pipeline

## Summary

The CLI (`ast push`) and GitHub build worker independently implemented the same build-push-register pipeline with duplicated component iteration logic (6+ copies) that had diverged: integration images used different naming (`-tool-` vs `-integration-`), the server never stripped secret defaults before registration, and agent image transforms behaved inconsistently. This refactoring extracts the shared logic into `astro-spec` and wraps both flows in a chainable pipeline struct so each process reads as a clear sequence of steps.

## Design

Three pure functions were added to `packages/astro-spec/pipeline.go`:

- **`CollectComponents(spec, agentName)`** — single source of truth for which components have build blocks and what they're named. Uses `-integration-` naming (matching the `integrations` spec key), fixing the CLI's prior `-tool-` convention.
- **`TransformSpecForRegistry(specMap, agentName, imageRefFn)`** — rewrites raw YAML map, replacing `build` blocks with `image` references. Takes a callback so CLI and server provide their own registry URL patterns.
- **`StripSecretDefaults(specMap)`** — removes default values from secret inputs. Previously CLI-only; now called by both paths.

Both the CLI and server wrap their flows in a pipeline struct with a private `step(fn)` method that centralizes error short-circuiting. Each public method is a thin wrapper that delegates to the shared functions or environment-specific logic:

```go
// CLI (apps/astro-cli/cmd/pipeline.go)
NewPushPipeline(ctx, cfg).
    ParseSpec().
    CollectComponents().
    Build().
    Push().
    TransformSpec().
    StripSecrets().
    Register().
    Err()

// Server (apps/astro-server/internal/githubbuild/pipeline.go)
NewGitHubBuildPipeline(ctx, cfg).
    FetchSpec().
    CollectComponents().
    CreateComponentRecords().
    RunBuildJobs().
    FetchReadme().
    TransformSpec().
    StripSecrets().
    Register().
    Err()
```

The server pipeline's `step` method also takes a name string and writes it to the DB for UI progress tracking.

## Migration

The integration image naming changes from `{agent}-tool-{name}` to `{agent}-integration-{name}`. A normal `ast push` (build + push) is unaffected since the build step creates images with the new name. `ast push --no-build` against locally cached images built with an older CLI will need a rebuild.
