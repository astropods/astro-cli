## Summary

`ast build` / `ast push` was intermittently failing on Docker Desktop 4.77.0 (Engine 29.5.3) with one of three symptoms — `archive/tar: invalid tar header`, `failed to read dockerfile: unexpected EOF`, or an indefinite hang at `[internal] load remote build context (10.0s, 15.0s, ...)`. All three were variants of the same daemon-side timer firing on a code path the CLI engages by default for cross-platform builds. This change keeps the CLI on the classic builder for the cases that don't actually need BuildKit, sidestepping that timer entirely.

## Design

The CLI's `buildImageSDK` (`apps/astro-cli/cmd/build_runner.go`) previously set `opts.Version = build.BuilderBuildKit` and opened a `/session` hijack whenever **either** secrets were declared **or** a platform was supplied:

```go
needBuildKit := platform != "" || len(secrets) > 0
```

Since every `ast build` invocation supplies a platform (resolved from the active server URL — `linux/amd64` for cloud, native for local), every build engaged BuildKit. With BuildKit-via-`/build` engaged, the daemon-side session manager runs a 5s-interval health check on our session's gRPC server (`buildkit/session/grpc.go:71-133`, `monitorHealth`); on the second consecutive failure (~10s) it cancels the session, which surfaces as one of the three symptoms above depending on what BuildKit was mid-doing.

We could not make those daemon-side health checks succeed from the client side. The fixes that did **not** work:

- Bumping `docker/moby`/`buildkit` module versions to match Desktop's bundled versions.
- Synchronizing the session dial with the build POST (a `sessionReady` barrier closed by the dialer).
- Registering `filesync` providers under `"context"` / `"dockerfile"` keys that BuildKit's dockerfile frontend uses.
- Wrapping the build context as an `io.ReadCloser` so net/http sees the close path.

In each case the build still failed at the +10s mark, with `sess.Run returned: <nil>` indicating a clean daemon-initiated close. The conn-byte traces showed ~422 bytes received over 10s — consistent with two `Health/Check` requests arriving and not being satisfied in time.

**The actual fix** is to stop engaging BuildKit for cases that don't require it:

```go
needBuildKit := len(secrets) > 0
```

The classic builder honors `opts.Platforms` directly, so cross-platform builds work fine without BuildKit. `opts.Platforms` is now set unconditionally when a platform is supplied; the BuildKit + `/session` code path runs only when build secrets are declared in the spec.

Other changes in the diff:

- A small barrier was retained for the BuildKit code path that orders `cli.DialHijack` before the build POST. We never observed the race it protects against, but the code shape (`go sess.Run(...)` followed immediately by `cli.ImageBuild(...)`) makes the ordering non-deterministic; the barrier is cheap and correct on its own merits. It does **not** address the monitorHealth lifecycle issue.

CLI patch bump: **0.15.2 → 0.15.3**.

## Migration

None. Builds that previously worked continue to work; builds that were failing intermittently on Docker Desktop 4.77.0 (no declared secrets) now succeed deterministically through the classic builder. Anyone hitting the tar-header / hang failure should `ast upgrade` (or rebuild) once `0.15.3` is published. Specs that declare `agent.build.secrets:` remain on the BuildKit code path and will hit the documented daemon-side timeout issue — no current agent in this org declares secrets, so this is dormant code, but it is not fixed by this PR.
