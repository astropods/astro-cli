## Summary

Renames the `AVATAR_S3_BUCKET` environment variable to `ASSETS_BUCKET` to align with terraform/infra.

## Design

Single env var rename with no behavior changes. Updated in three places:

- **`internal/config/config.go`** — `AvatarConfig.S3Bucket` now reads from `ASSETS_BUCKET`.
- **`cmd/backfill-avatars/main.go`** — CLI usage comment and `os.Getenv` call updated.
- **`docs/changelog/fix/profile-image-bugs-2026-03-20.md`** — Historical changelog corrected to reflect the new name.

## Migration

In deployed environments, rename the environment variable: `AVATAR_S3_BUCKET` → `ASSETS_BUCKET`.
