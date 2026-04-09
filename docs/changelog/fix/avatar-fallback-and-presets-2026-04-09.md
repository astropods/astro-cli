# Deterministic avatar URLs with generated identity fallback

## Summary

Blueprint and deployment avatars were broken after the avatar_version removal: agents without custom avatars showed broken images, cross-account deploys didn't copy avatars, and user preset assignments stored SVG content at JPEG keys causing silent render failures.

## Design

**Client-side deterministic URLs.** All avatar rendering now constructs URLs client-side from known identifiers — `getAgentAvatarUrl(account, name)` for blueprints and `getDeploymentAvatarUrl(id)` for deployments. Components no longer rely on `avatar_url` from API responses to decide what to render.

**`BlueprintIdentity` with `onError` fallback.** The component loads the deterministic URL as an `<img>`. If it 404s (no custom avatar uploaded), it falls back to the procedurally generated SVG identity. An optional `url` prop allows deployment contexts to override with their own avatar path.

**`useCardAvatar` hook for trading cards.** SVG `<image>` elements don't support `onError`, so the hook probes the image URL via `new Image()` before the card is generated. Returns `null` while loading, then a stable `CardAvatar` (`{ url }` or `{ svg }`) — the card SVG is only generated once the avatar is resolved, preventing flickering.

**Cross-account deploy avatar copy fix.** `CopyAgentToDeployment` was using the target (deploying) account to locate the blueprint avatar instead of the source (blueprint owner) account. Added `sourceAccountName` to `deployContext` so the copy uses the correct S3 path.

**JPEG preset backfill.** The 25 preset placeholder SVGs have been pre-rendered as JPEGs. `presetKey` now points to the JPEG versions so `SetPreset`/`AssignPreset` copies a real JPEG to the `.jpg` avatar key. The backfill worker validates content type via `HeadObject` and re-assigns any presets that have the wrong content type (SVG-in-JPEG from the previous backfill).

## Migration

- **Backfill runs automatically.** The daily River job will detect and fix broken preset avatars (SVG content type at .jpg keys) on next run.
- **Asset sync required.** The new JPEG preset files in `assets/placeholders/accounts/` must be synced to S3 before the backfill can use them. This happens automatically on merge via the `sync-assets` workflow.
