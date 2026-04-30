## Summary

Adds persistent "Share on LinkedIn" and "Share on X" buttons to the README section header of public blueprint detail pages, so users can one-click share a blueprint with a pre-composed post.

## Design

The buttons appear on the right end of the ReadMe section header bar, only when the blueprint is public and published (`isPublic = true`). They use `Button variant="ghost" size="xs"` — same tokens as the rest of the header bar — and open the native platform share composer in a new tab.

**LinkedIn** uses the standard URL-sharing endpoint:
```
https://www.linkedin.com/sharing/share-offsite/?url={encoded_url}
```
LinkedIn auto-generates the preview card from the og:image (1200×630 badge) already in place.

**X** uses the tweet intent with a pre-filled message:
```
https://x.com/intent/tweet
  ?text={description} — {account}/{name} on Astro AI
  &url={encoded_url}
```
The description is the blueprint's agent_card description (falls back to the agent name if missing). X shows the URL as an unfurl card.

The X mark logo is inlined as an SVG path (lucide-react does not ship a social X icon).

`BlueprintDetailContent` accepts two new optional props: `description?: string` and `isPublic?: boolean`. The `BlueprintDetail` page passes both from loader data.

## Migration

No action required.
