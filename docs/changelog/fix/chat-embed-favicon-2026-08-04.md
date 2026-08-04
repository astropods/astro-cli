# Fix blurry favicon in the CLI-embedded chat UI

## Summary

The chat tab served locally by the CLI (`ast dev` → the embedded chat UI) showed a
blurry favicon, while the same chat in production was crisp. Both are the same
`astro-client` chat UI, so the tab icon should look identical in either place.

## Design

The full app declares its favicons through React Router's `<Links>` (in `root.tsx`),
which are server-rendered into the initial HTML — so production loads the crisp
32×32 PNG immediately. The chat-embed build, however, is a static SPA with no SSR:
`<Links>` only runs after hydration, so the static shell declared no icon at all and
the browser fell back to the default low-res `/favicon.ico`, upscaling it (blurry on
hi-DPI/retina tabs).

The fix declares the favicon `<link>`s directly in the static `chat-embed.html`
shell, mirroring `root.tsx`'s set (`apple-touch-icon` 180, 32×32, 16×16, manifest),
so the browser loads the crisp 32×32 PNG from the initial HTML — matching production.
The favicon assets already ship in the build output (copied from `public/`); only the
`<link>` declarations were missing.

## Migration

None.
