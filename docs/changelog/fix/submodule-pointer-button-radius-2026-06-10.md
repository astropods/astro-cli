# Fix website submodule pointer and restore Button border radius

## Summary

A bulk submodule bump regressed `modules/website` to the same SHA as `modules/blog`, so `update-submodules.sh` cannot honor the recorded pointer and leaves the working tree ahead of HEAD. Separately, the deployment chat UI PR overwrote the shared `Button` and `Tooltip` primitives with stock shadcn styling, affecting the whole app.

## Design

**Submodule pointer** — Point `modules/website` back at `58585d11` on `astro-ai-website` `main` (the commit from #1300). The invalid `6dd37ee` SHA belongs to `astro-website` (blog) and is rejected by the website remote.

**Button / tooltip isolation** — Fully revert `Button` and `Tooltip` to their pre-chat design-system definitions. Chat keeps assistant-ui shadcn-style controls via a scoped `ChatButton` (`assistant-ui/chat-button.tsx`) and chat-only tooltip overrides on `TooltipIconButton`.

## Migration

Nothing required. After merge, `bash scripts/update-submodules.sh` should complete without a divergent `modules/website` warning.
