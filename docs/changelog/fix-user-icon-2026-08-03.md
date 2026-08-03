# Review queue user avatars

## Summary

Review queue inputs now show the user avatar associated with each trace instead of always showing a generic user icon.

## Design

The review queue response preserves the trace user identifier and resolves user details through the same batched Astro and Slack identity hydration used by observability views. The queue preview uses those details to render the matching avatar, with the generic user icon retained as a fallback when no profile is available.

## Migration

No migration is required.
