# Local-dev: avatar wiring and image-preflight fixes

## Summary

Two regressions surfaced by the new local-dev script (#920), which introduced
Traefik on port 80 and shipped without avatar config in `.env.example`:

- `GET /api/v1/deployments/summary` panicked with a nil pointer dereference
  when avatar storage was unconfigured.
- `POST /api/v1/deploy` rejected every deploy with `422 image_not_found`
  because Traefik now answers 404 for `Host: registry.localhost`, and the
  registry preflighter (#864) treated the 404 as a missing manifest.

## Design

**Avatars in local dev.** `.env.example` now sets `ASSETS_LOCAL_DIR=../../assets`
and `ASSETS_CDN_URL=http://localhost:8080/assets` so a fresh checkout has avatar
storage wired up. When `cfg.Avatar.IsLocal()` is true, the server registers
`router.Static("/assets", LocalDir)` so URLs of the form `{ASSETS_CDN_URL}/{key}`
resolve from the on-disk `assets/` dir without a real CDN — production keeps
using S3 + CloudFront and never hits this route. `ListDeploymentsSummary` now
nil-guards `avatarStore` before dereferencing it, matching the pattern already
used elsewhere in `handlers/deploy.go`. The handler degrades gracefully (no
avatar URL in the response) instead of panicking when avatars are unconfigured.

**Image preflight.** The preflighter HEADs the registry to fail fast on missing
manifests. `ast-dev push` doesn't actually push anywhere — it retags local images
as `registry.localhost/<ns>/<agent>:<tag>` so kubelet resolves them from the
Docker daemon via `imagePullPolicy: IfNotPresent`. Before #920 this worked
because nothing was listening on `:80`, so the preflighter's HEAD got a transport
error and failed open. Traefik's catch-all converted that into a 404 that the
preflighter rejected. The fix short-circuits `*.localhost` / `localhost` /
`127.*` hosts before any HTTP call — those names are reserved for local
development by RFC 6761 and are used purely as a kubelet-resolvable tag scheme
in this codebase. Real registries (ECR, etc.) are unaffected.

## Migration

None required. Existing `.env` files without the new `ASSETS_*` vars continue
to work because the avatar code paths nil-guard, and the preflight short-circuit
for `.localhost` hosts is unconditional. Adding the two `ASSETS_*` lines is
optional and only needed to exercise avatar URLs locally.
