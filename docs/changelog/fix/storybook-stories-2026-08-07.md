# Fix stale Storybook stories

## Summary

A headless sweep of all 425 Storybook entries in `astro-client` found a batch that errored on load, all of it reference and fixture drift: a docs page pointing at story exports that no longer exist, an integration marked `known` so the component fetched an icon that was never shipped, the in-dialog build-log stories missing the accessible name Radix requires, two theme stories keying list children on an anonymous fragment, and a colour diagnostics panel calling a `setColors` setter that no longer exists. Typecheck could not see that last one, because `tsconfig.app.json` excludes the diagnostics files to keep the react-three-fiber JSX augmentation out of the app's type graph. That exclusion had a second consequence worth naming: esbuild fell back to the root `tsconfig.json`, which set no `jsx`, so those files compiled against the classic runtime and only rendered because of a stray `React` import. The root config now declares `react-jsx` so the fallback matches the app.

## Design

Avatars needed a separate call. `apps/astro-client/public/assets` symlinks the repo-root `assets/` directory, so Vite — and therefore Storybook — serves `/assets/avatars/**` in a normal checkout. But that tree is gitignored per-developer backfill output, so it is simply absent in a fresh worktree and in CI, which makes an avatar 404 there an environment gap rather than an app bug. The app already treats a missing avatar as a normal state: `UserAvatar` and `AvatarImage` swap to the shared placeholder on error, and story fixtures using invented handles exercise exactly that path. The rule that follows is that fixtures cannot durably point into `/assets/avatars/**`; a story that needs a real image passes an explicit URL into the committed `assets/placeholders/` set. The live-reveal overlay story now does, because its avatar feeds a trading-card SVG with no error fallback of its own, and the colour diagnostics panel samples the placeholder set rather than local agent avatars, which also drops Storybook's only external network dependency.

## Migration

None. Storybook only.
