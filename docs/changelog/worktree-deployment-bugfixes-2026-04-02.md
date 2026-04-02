# Remove skeleton loader from blueprint detail page

## Summary

The blueprint detail page occasionally flashed a skeleton loader despite being fully server-rendered. This happened when the server loader's fetch failed (returning null), causing `useBlueprint` to start a client-side fetch with `isLoading: true` before inevitably landing on an error or not-found state.

## Design

Removed the `isLoading` check and `BlueprintDetailSkeleton` component entirely. Since React Router runs the loader before rendering, `loaderData` is always available at mount time. If the server couldn't load the blueprint, the page now falls directly to the "Agent not found" state instead of briefly showing a skeleton.

## Migration

No migration required.
