# Port identitygen to Go and serve blueprint avatars from S3

## Summary

Agent placeholder avatars used to be generated client-side by `packages/astro-identity-gen` (TypeScript). The client would try to load a CDN image, and on any 404 fall back to rendering a procedural SVG via `dangerouslySetInnerHTML`. This produced load-order complexity, made avatars un-cacheable (inline SVG defeats the HTTP layer), and maintained two avatar paths — uploaded vs generated — throughout the client.

This change moves the generator to Go, runs it on the server at blueprint creation time, and stores the result in S3 like any other avatar. The client becomes a plain `<img>` with a single static fallback URL. The TypeScript package is deleted.

## Design

### Go port (`apps/astro-server/internal/identitygen/`)

The Go package is a straight port of the TS `generateIdentity`/`generateCustomIdentity` logic. Parity is enforced in two tiers:

- **Exact, bit-for-bit:** `hash()` and the Mulberry32 `rng` sequence. A single-bit drift would change which palette or edge-style the `Math.floor(rng() * n)` picks select, producing a qualitatively different identity.
- **Tolerant:** Geometric float output (rotation, radius, path coordinates). Absorbs last-ULP drift between `Math.cos/sin` and Go's `math.Cos/Sin`.

Test parity is enforced against 5 diverse seeds captured from the (now-deleted) TS reference: `testdata/hash_rng.json` stores IEEE-754 bit patterns for bit-exact checks; `testdata/choices.json` stores every decision the generator makes per seed, with exact equality on discrete fields and `1e-9` tolerance on floats.

### Rasterization

The server stores JPEG, not SVG. Three reasons: match the account-avatar format already in S3, avoid mis-labelling objects (SVG content under a `.jpg` key is confusing), and get a single cache/transform story at the CDN.

Rasterization literally parses the SVG the generator emits and draws it — the rendering logic is not duplicated on a 2D graphics API. `github.com/srwiley/oksvg` + `rasterx` handle the parse/raster pass in pure Go. One accommodation: pure-Go SVG parsers don't support CSS `oklch()`, so the palette data carries sRGB hex strings for SVG embedding. Hex is derived from OKLCH via inlined conversion math in `gen-theme.ts` — same design-system source, different encoding.

The rasterizer supersamples 2× and downsamples with Catmull-Rom (same filter `avatar.processImage` uses for user uploads), then JPEG-encodes at quality 92. This gives clean edges on the flat-color geometric shapes.

### Blueprint creation

`CreateBlueprint` now generates a JPEG via `identitygen.GenerateIdentityJPEG` and uploads it with a new `avatar.Store.WriteAgentAvatarJPEG` that writes pre-encoded bytes directly (bypassing `processImage`, which would decode and re-encode our own output). Generation failures log a warning but do not fail blueprint creation — the backfill job catches them.

### Backfill worker

`BlueprintAvatarBackfillWorker` is a new River periodic worker (24h + `RunOnStart: true`) that mirrors the existing `AvatarBackfillWorker` structure. Two passes in one run:

1. Cursor-paginate `agents`, skip if S3 has the avatar, otherwise generate + upload.
2. Cursor-paginate `deployments`, skip if S3 has the avatar, otherwise copy from the (now-guaranteed) blueprint avatar via `CopyAgentToDeployment`.

### Client

`BlueprintIdentity.tsx` collapses from ~60 lines of probe-and-fallback state machine to a single `<img>` with an `onError` fallback to `getFallbackAvatarUrl()` (the static placeholder, same pattern as `UserAvatar.tsx`). The component's public API is unchanged, so the 14 call sites need no edits. `useCardAvatar` was a probe-and-generate-on-404 hook; it's deleted in favor of callers passing `{ url }` directly.

### Deployment avatar URLs and cache busting

`ListDeployments` and `GetDeployment` now populate `AvatarURL` on response objects via `avatarStore.DeploymentAvatarURL(id)`. Previously these handlers never set the field, so the configure panel always fell back to the blueprint avatar instead of the deployment's own avatar. `DeployFormFields` now passes the `url` prop through to `BlueprintIdentity` so the override is actually used.

Deployment avatar cache busting mirrors the existing blueprint pattern: `bustDeploymentAvatar(id, blob)` and `useDeploymentAvatarBust(id)` in `avatar-bust.ts`. After upload, the blob URL immediately overrides the CDN URL across all components (`DeployedAgentCard`, `ActiveDetailView`, `LiveRevealOverlay`, `KebabMenu`, `ConfigurePanel`, `ConfigureDeployment`) without requiring a page refresh.

### Reveal overlay stability

The deploy reveal overlay (`LiveRevealOverlay`) was reactive to the deployments query — it rebuilt its deployment object from query data as it loaded, causing the avatar URL to change mid-animation. This triggered color re-extraction which briefly hid the card. The reveal deployment is now built once from location state via a `useState` initializer, making the overlay static and immune to background data loading.

The holo card effect also had stale CSS variables on pointer leave: `HOLO_RESET_VARS` only reset `--o`, `--rx`, `--ry` but left `--px`, `--py`, `--fl`, `--ft`, `--fc` at their exit-point values, causing shine/glare gradients to stick. All 8 variables are now reset. The reveal card border radius was also mismatched (24px via `rounded-2xl` vs 16px in the share modal) and now uses `borderRadius: 16` to match.

### Cleanup

`packages/astro-identity-gen/` and all workspace references are gone: `apps/astro-client/package.json`, `deployment/Dockerfile.astro-client`, `.github/workflows/{test,deploy-preview}.yml`, root `README.md`, root `CLAUDE.md`, and the client's `moon.yml` deps. `packages/astro-trading-card/dev-client.ts` had an incidental dev-only import; its sample avatars are now inline static SVGs.

## Migration

- **Deploy order:** server first, then client. The backfill runs on server startup (`RunOnStart: true`), so the vast majority of blueprints have avatars by the time the new client rolls out.
- **Worst-case race:** new client hits an existing blueprint whose avatar hasn't been backfilled yet → `<img>` 404s → `onError` swaps to the static fallback placeholder. Resolves itself on the next backfill tick.
- **Uploaded custom avatars:** unchanged. Same `UploadAgent` path, same key, same URL.
- **Manual backfill:** not needed. The River periodic worker handles it automatically on startup and every 24h.
