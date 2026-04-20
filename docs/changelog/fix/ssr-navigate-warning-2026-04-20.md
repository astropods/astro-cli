---
title: Fix SSR warning for unconditional `<Navigate>` redirect pages
---

## Summary

The dev server was emitting a React Router warning on every request that hit a redirect-only route:

> `<Navigate>` must not be used on the initial render in a `<StaticRouter>`. This is a no-op, but you should modify your code so the `<Navigate>` is only ever rendered in response to some user interaction or state change.

With SSR enabled in `react-router.config.ts`, the server uses `<StaticRouter>`. Components that render `<Navigate>` on their first render are no-ops server-side — the browser has to perform the redirect after hydration, which is both incorrect (a user-interaction-only API used during render) and wasteful (server does a full render, then the client immediately navigates away).

## Design

Index/redirect routes now do their redirects in a `loader` using `redirect()` from react-router, instead of rendering `<Navigate>` in the component:

```tsx
import { redirect } from "react-router";

export async function loader() {
  return redirect("/blueprints");
}

export default function RedirectForIndex() {
  return null;
}
```

The loader runs on the server and returns a `Response` with a 302, so the component never renders and `<StaticRouter>` never sees a `<Navigate>`. On client-side navigations, React Router still runs the loader and follows the redirect before attempting to render, so the end behavior matches the old `<Navigate>` version.

This is applied to the five routes whose only job was to redirect unconditionally:

- `/` → `/blueprints`
- `/blueprints` → `/blueprints/discover`
- `/settings` → `/settings/account`
- `/organization` → `/organization/new`
- `/:account/agents/:deploymentId/configure` → `.../configure/deployment`

Remaining `<Navigate>` call sites (`root.tsx`'s `OnboardingGuard`, `ProtectedLayout`, `Login`/`Signup`, `blueprints/Personal`, `blueprints/AccountBlueprints`) are all gated on `!isLoading && <auth condition>` and so return `null`/children on the SSR initial render — they don't trigger the warning and are left as-is, since they depend on client-only auth state.

## Migration

None. End-user routing behavior is unchanged.
