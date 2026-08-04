# Activate agent-card animation on demand

## Summary

The Agents page activated every decorative star animation while hydrating, even though idle cards immediately set their playback rate to zero. Local profiling of a 50-card page registered more than 8,000 running animations and reproduced a roughly 10-second main-thread stall.

Star animations now remain paused until their card is hovered. This removes the page-load freeze without changing the server-rendered cards or their hover experience.

## Design

Each card discovers and caches its own drift and twinkle animations on first hover. The existing acceleration and coast-down remain unchanged, but only the hovered card enters the browser's running animation registry. When coast-down completes, its animations pause again while preserving their final frame. Cards that are never hovered do not call `getAnimations()` or `play()`.

This is a targeted activation fix. It does not change the star count or SVG markup, so a future rendering optimization can reduce the remaining HTML and DOM weight independently.

In the same 50-card Chromium profile, the longest startup main-thread task fell from approximately **10,000 ms before the fix to 187 ms after it**. Every idle animation remained paused, one hovered card ran 168 animations, and all animations returned to paused after coast-down.

## Migration

No migration is required.
