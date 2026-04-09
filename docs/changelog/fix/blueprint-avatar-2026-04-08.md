# Replace avatar version counter with HTTP-native cache invalidation

## Summary

Avatar cache-busting used a `avatar_version` integer counter stored in the database and appended as `?v=N` to URLs. Every component rendering an avatar had to receive this version via props — any component that forgot (like the blueprint detail author sidebar) silently showed stale avatars. This replaces the entire mechanism with standard HTTP caching and fixes a separate bug where agent blueprint avatars were broken in production due to a misconfigured environment variable.

## Design

**HTTP-layer caching replaces the version counter.** S3 avatar uploads now set `Cache-Control: public, max-age=60, stale-while-revalidate=86400`. S3 auto-generates ETags, so browsers and CloudFront revalidate with conditional requests after 60 seconds. The existing CloudFront config (`min_ttl=0`) already respects origin cache headers, so no infrastructure changes are needed.

**`UserAvatar` is self-sufficient.** It only needs a `handle` prop — no more `avatarVersion` prop drilling through ~11 component locations. Avatar URLs are deterministic (`/avatars/{handle}.jpg`), and the HTTP layer handles freshness.

**Instant post-upload feedback via blob URLs.** After uploading, `bustAvatar(handle, blob)` stores a local `blob:` URL that `UserAvatar` renders directly from memory — zero network requests, completely bypassing both the browser cache and CloudFront. On next page load, the CDN takes over.

**`avatar_version` column dropped entirely.** The column previously served double duty as a cache-buster and a "has custom avatar" flag. Cache-busting is now HTTP-native, and avatar existence checks use S3 `HeadObject` where needed (auth login, account renames, agent transfers — all rare operations). Agent and deployment API responses now always include `avatar_url` since the URL is deterministic.

**Agent avatar CDN URL fix.** The server config was reading `ASSETS_URL` but Helm sets `ASSETS_CDN_URL`, causing agent avatar URLs to fall back to relative `/assets/` paths in production (broken images on blueprint pages).

## Migration

- **Database:** The `avatar_version` column is dropped from `accounts`, `agents`, and `deployments` tables. Atlas will handle this automatically from the schema diff.
- **No infrastructure changes required.** CloudFront already respects origin `Cache-Control` headers.
