## Summary

Profile images were inconsistent across the app — different fallback avatars appeared in different places, WorkOS profile pictures weren't always passed through, and there was no upload path. This replaces the fragmented client-side avatar system with a unified CDN-backed approach where every account always has exactly one image in S3.

## Design

### Architecture

Every account has a single avatar file at `avatars/{handle}.jpg` in the assets S3 bucket (or local `assets/` dir in dev). The client renders avatars with one URL pattern — `{CDN}/avatars/{handle}.jpg?v={version}` — and the `UserAvatar` component is the single entry point. No runtime fallback logic exists anywhere.

**Lifecycle:**
1. Account created → server copies a deterministic preset SVG to `avatars/{handle}.jpg`
2. OAuth signup with profile pic → server ingests the photo, overwriting the preset
3. User uploads custom photo → overwrites via `POST /api/v1/accounts/:account/avatar`
4. User reverts → picks a preset via `PUT /api/v1/accounts/:account/avatar/preset/:index`

### Server changes

- **`internal/avatar/`** — New package with a `Backend` interface (`S3Backend` for production, `LocalBackend` for dev) and a `Store` that handles upload, resize, preset assignment, ingestion, move, and delete.
- **`handlers/avatar.go`** — `POST /avatar` (upload), `PUT /avatar/preset/:index` (pick preset), `DELETE /avatar` (reset to default).
- **`accounts` table** — New `avatar_version` column for cache-busting. Incremented on every avatar change; the client appends `?v={version}` to the CDN URL.
- **Auth callback** — On OAuth login, if the WorkOS user has a profile picture and the account's `avatar_version` is 0, the photo is ingested into S3 in a background goroutine.
- **Config** — `ASSETS_BUCKET` for production S3, `ASSETS_LOCAL_DIR` for local filesystem dev, `ASSETS_URL` for CDN base URL.

### Client changes

- **`UserAvatar`** component simplified to three props: `handle`, `name`, `avatarVersion`. Uses `getAvatarUrl()` to build the CDN URL and `onError` to fall back to a shared placeholder if the image fails to load.
- **All call sites** standardized to the same inline pattern — no more helper functions, no more passing `profilePictureUrl` around.
- **Deleted:** `presetAvatars.ts`, `PresetAvatarPicker` component, `getUserAvatarProps` helper.

### Backfill

`cmd/backfill-avatars/` is a standalone CLI that assigns preset avatars to all existing accounts. Supports both S3 and local filesystem backends, dry-run mode, and is idempotent (skips accounts that already have an avatar).

### Asset sync

The S3 sync workflow now uses an allowlist (`--include "integrations/*" --include "placeholders/*"`) instead of a blocklist, so only explicitly listed directories are synced and user-uploaded avatars are never touched.

## Migration

1. Apply schema: `avatar_version integer NOT NULL DEFAULT 0` on the `accounts` table (Atlas will diff automatically).
2. Set `ASSETS_BUCKET`, `ASSETS_URL` env vars in deployed environments.
3. Run the backfill: `DATABASE_URL=... ASSETS_BUCKET=... go run ./cmd/backfill-avatars`
4. Deploy server and client together — the client expects `avatar_version` in account API responses.
