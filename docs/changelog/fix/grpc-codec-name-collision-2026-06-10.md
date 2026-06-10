# Fix BuildKit on Docker Desktop 4.77.0

## Summary

`ast build` started failing on Docker Desktop 4.77.0 (Engine 29.5.3) with one of three symptoms:

- `archive/tar: invalid tar header`
- `failed to read dockerfile: unexpected EOF`
- indefinite hang at `[internal] load remote build context`

It looked like a Docker daemon regression. It wasn't. There were **two of our own bugs** stacked. Both are fixed here, and the `#1319` BuildKit-skip workaround is reverted now that they no longer mask each other.

## Bug 1 — gRPC codec name collision

`packages/astro-proto/admin/v1` and `packages/astro-proto/connect/v1` each register a JSON-over-gRPC codec in their `init()`. The codec lied about its identity:

```go
func (jsonCodec) Name() string { return "proto" }
```

`google.golang.org/grpc/encoding.RegisterCodec` keys the global registry by `Name()`. Registering as `"proto"` silently replaced gRPC's default proto codec **process-wide** for every binary importing the package.

`astro-cli` imports `connect/v1` (for device-connect), so inside `astro-cli`:

1. `connect/v1`'s `init()` runs at process start.
2. gRPC's default proto codec is now `json.Marshal` / `json.Unmarshal`.
3. `ast build` invokes BuildKit's `session.NewSession`, which registers `grpc_health_v1.HealthServer`.
4. That server inherits the hijacked codec.
5. The daemon sends a protobuf-encoded `HealthCheckRequest`; our server tries `json.Unmarshal` on the bytes → `unexpected end of JSON input`.
6. Two consecutive Internal errors → daemon tears the session ~10s in.

**Fix:** rename the codec to `"json"` (its true name) and have the admin/connect *clients* opt in via `grpc.WithDefaultCallOptions(grpc.CallContentSubtype("json"))`. Any other gRPC server in the same process (BuildKit's session, etc.) gets the real built-in `"proto"` codec back. astro-server side needs no change — gRPC selects the codec from the request's content-subtype at dispatch.

## Bug 2 — legacy `/build` + `/session` path is structurally fragile

With the codec collision fixed, a *second* failure surfaced in Engine 29.5.3: the daemon-side session ClientConn re-dialed and hit buildkit's single-shot dialer with `only one connection allowed`, killing the session ~5s into every build.

Tracing showed a goroutine race in `buildkit/session.Run` — the dialer returns the conn, then `sess.Run` assigns `s.conn` and calls `http2.Server.ServeConn(conn)` in a separate goroutine. Our barrier on `sessionReady` was signaling the moment the dialer returned, *before* `ServeConn` started reading. `ImageBuild`'s POST `/build` was racing the http2-server startup. We tried two narrow fixes — a `readyOnRead` wrapper, then a `readyOnWrite` wrapper — and confirmed each partially closed the race, but neither was reliable across pre-pull sizes and goroutine-scheduling jitter.

Worse, even when the session stayed alive, the `[internal] load remote build context` step would hang for minutes on Docker Desktop 4.77.0 with the legacy `POST /build` endpoint. `docker buildx build` against the same context finished the *entire* build in 20 seconds. The legacy endpoint is structurally broken on this engine version.

**Fix:** drop the legacy path entirely. Migrate to `bkclient.Solve` against the Docker daemon's embedded BuildKit gRPC endpoint — the same path `docker buildx build` uses with the `docker` driver.

```go
bkc, err := bkclient.New(ctx, "", bkclient.WithContextDialer(
    func(ctx context.Context, _ string) (net.Conn, error) {
        return dockerCli.DialHijack(ctx, "/grpc", "h2c", nil)
    }))
// ...
bkc.Solve(ctx, nil, bkclient.SolveOpt{
    Frontend:      "dockerfile.v0",
    FrontendAttrs: map[string]string{"filename": dockerfile, "platform": platform, ...},
    LocalMounts:   map[string]fsutil.FS{"context": contextFS, "dockerfile": contextFS},
    Session:       []session.Attachable{secretsprovider.FromMap(...)},
    Exports:       []bkclient.ExportEntry{{Type: "moby", Attrs: {"name": tag}}},
}, statusCh)
```

This bypasses every problem we ran into:

- **No codec collision blast radius into BuildKit's session** — `bkclient.Solve` opens its own gRPC connection; the buildkit `grpc_health_v1` server on the session never touches the hijacked conn.
- **No `ServeConn`-vs-`SETTINGS` race** — gRPC's `Dial` does the HTTP/2 handshake cleanly on a fresh conn, not over an HTTP/1.1 hijack.
- **No single-shot dialer** — the `only one connection allowed` guard exists specifically in the `/session` bridge.
- **Sane context transfer** — context, dockerfile, and secrets all flow over the same gRPC connection as proper filesync streams. The `[internal] load remote build context` step completes in milliseconds instead of timing out.
- **Native cross-arch** — BuildKit's QEMU emulation just works; we no longer need the `#1319` BuildKit-skip workaround.

### Sub-bug 2a — exporter must be `moby`, not `image`

A subtle gotcha: BuildKit's `"image"` exporter writes to the BuildKit-internal containerd namespace, which is *not* the same store the classic Docker API reads from. `docker images` (the new format with `DISK USAGE` / `CONTENT SIZE` columns) sees these images, but `docker image inspect <tag>` and `docker tag <tag> …` fail with `No such image`. That would have silently broken `ast push`, which uses `dockerCli.ImageTag` + `dockerCli.ImagePush`.

The undocumented `"moby"` exporter writes to the daemon's moby image store directly, making the image visible to the classic API. Buildx does this transformation in `build/opt.go:466`:

```go
if e.Type == "image" && nodeDriver.IsMobyDriver() {
    opt.Exports[i].Type = "moby"
```

The `moby` constant doesn't appear in `client.Exporter*` public constants — it's discovered only by reading buildx source. The build-output signature is the giveaway: with `moby`, the log shows an extra `unpacking to docker.io/library/<tag> done` step.

### Result

End-to-end verified on Docker Desktop 4.77.0 (Engine 29.5.3):

| Scenario | Time |
|---|---|
| Trivial alpine build (native arm64) | 2s |
| Cross-arch `linux/amd64` from arm64 | 3s |
| Multi-stage bun build, 388 npm packages | 18s |
| `docker image inspect <built-tag>` | works |
| `docker tag <built-tag> <new-tag>` | works |

The legacy code path is deleted in full: `buildImageSDK`, `prePullBaseImages` (BuildKit handles base images via its own snapshotter), `parseDockerfileBaseImages`, `readDockerignore` (the dockerfile frontend reads it from the synced context), `streamBuildOutput` (replaced by `progressui.DisplaySolveStatus`), and the `readyOnRead`/`readyOnWrite` race-mitigation wrappers — net `-721/+75` lines.

## Why now

Bug 1 was latent since 2026-02-27 (codec introduced) and weaponized 2026-03-06 (`astro-cli` started importing the package). Both bugs were dormant until Docker Desktop 4.77.0 / Engine 29.5.3 (2026-06-08) tightened the session-side health check and sub-channel timeouts. Same broken code, different daemon behavior — a latent pair tipped into a visible failure.

## Timeline

| Date | Commit / event | What changed |
|---|---|---|
| 2026-02-27 | `9604e0d6e` | Codec collision introduced in `admin/v1/admin.pb.go`. Dormant — only `astro-server` imports it. |
| 2026-03-06 | `3e26eabef` | Same broken codec copied into `connect/v1/connect.pb.go`. Still dormant. |
| 2026-03-06 | `b32795d5b` | **Bug 1 fuse lit.** `astro-cli/internal/connect/{client,exec}.go` imports `connect/v1`. Every `astro-cli` binary now ships the hijacked codec. |
| 2026-06-08 | Docker 4.77.0 | Engine 29.5.3 tightens session-side health checks and transport timeouts. Latent bugs detonate. |
| 2026-06-10 | `745fb1b9f` | Symptom-level workaround merged: skip BuildKit when no secrets. Re-broke cross-arch builds on Apple Silicon. Reverted by the migration. |
| 2026-06-10 | this PR | Codec renamed; legacy `/build` path deleted in favor of `bkclient.Solve` against the daemon's embedded BuildKit; switched to the undocumented `moby` exporter so the built image is visible to the classic Docker API (`ast push`). |

## How we got here

The codec collision was the easier of the two to find — daemon log showed `unexpected end of JSON input` during a health check, which is unambiguously a codec mismatch, not a network or session issue.

The session-conn race was harder. After fixing the codec we tried bumping module versions, pre-registering filesync providers, wrapping the request body, and synchronizing the `/session` dial with the `/build` POST. None worked reliably. A diagnostic `net.Conn` logging shim accidentally *masked* the race because per-call `fmt.Fprintf` overhead bought `ServeConn` enough time to start before the daemon's health check fired — that heisenbug pointed straight at the goroutine race in `buildkit/session.Run`. Two narrower mitigations (signal on first `Read`, then first `Write`) each closed the window partially but neither held across pre-pull-heavy builds and goroutine-scheduling jitter.

The migration to `bkclient.Solve` short-circuited the whole investigation: the legacy `/build` endpoint has too many sharp edges on Engine 29.5.3, and Docker has clearly shifted investment to the buildx path. The PR's history (`6c8051f78` codec → `dcefc2cf5` race wrapper → `2602ce184` migration) preserves the path so the next person doesn't repeat it.

## Rollout

The codec rename in Bug 1 is a coordinated client/server change. Mismatched versions across the wire will fail:

- `astro-server` admin gRPC ↔ `astro-queen` — must deploy together.
- `astro-server` fleet gRPC ↔ `ast connect` — users on old CLI will fail until they upgrade.

Both are internal services. No external surface area.

Bug 2's fix (`ast-build`'s switch to `bkclient.Solve`) is `astro-cli`-only.

## Preventing regression

For Bug 1, two layers:

1. **Unit tests** — `packages/astro-proto/admin/v1/codec_test.go` and `packages/astro-proto/connect/v1/codec_test.go` each assert `jsonCodec{}.Name() == "json"`. Direct calls on the codec type, no dependency on init order or registry state. Any rename fails the test with a message pointing back to this RCA.

2. **CI grep guard** — `.github/workflows/test.yml` adds a `codec-collision-guard` job that runs unconditionally and rejects any `Name() string` method in `packages/astro-proto/` returning a built-in gRPC codec name (`"proto"`, `"gzip"`, `"identity"`). Catches the same footgun in a *new* proto package before any tests run.

For Bug 2, the structural fix is the deletion. Once the legacy `/build` path is gone, no future code can race the session shim. The `bkclient.Solve` path is the same one `docker buildx` uses; if it regresses, the whole Docker ecosystem will notice.

## Lessons

- Never name a custom gRPC codec after a built-in. Pick a unique name; have clients opt in via `CallContentSubtype`.
- `init()`-time global registration is a sharp tool. Anything that imports the package inherits the side effect, including transitive imports the package author can't see.
- When a gRPC subsystem fails with codec-shaped errors (`unmarshalling`, `unexpected end of input`), suspect a global codec collision before blaming the transport.
- A repro that works in a sibling binary but not the one you care about is a strong signal the difference is *in your binary*, not the environment.
- When adding diagnostic logging *fixes* a bug, you've found a race. Trace the timing change and locate the synchronization point you're missing.
- Two stacked bugs that mask each other are hard to root-cause in one pass. Removing the outer one will surface the inner one — keep that expectation explicit during incident triage.
- When a vendored protocol has more than one design-flaw on the same path, *stop patching it*. The legacy `/build` endpoint had codec hijack risk, session-conn race, single-shot dialer, and slow daemon-side processing all on the same code path. Each one was individually fixable; the combination was not. Migrating to the path the upstream actually maintains was the right answer earlier than we admitted.
- "Visible to `docker images`" ≠ "visible to `docker image inspect <tag>`". On a containerd-backed daemon, those two commands read from *different namespaces*. When migrating off the classic Docker API, validate the entire downstream chain (in our case, `ast push`) on a real image, not just the build itself.
