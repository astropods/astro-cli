# Profile Upload Feedback

## Summary

Profile and organization avatar uploads now update visibly as soon as the upload succeeds. The previous flow relied on query invalidation and CDN/session refresh timing, so a successful upload could still look stale in the profile page, header, or organization switcher.

## Design

Account avatar uploads now mirror the blueprint avatar flow:

- The upload mutation patches the account detail cache with the server-returned `avatar_url` and palette.
- Profile edit surfaces apply a session-local blob override from the cropped image for immediate feedback.
- Auth account data refreshes after upload so global chrome consumers receive the same versioned avatar URL.

The server remains the source of truth for persisted avatar URLs and cache-busting tokens; the local blob only covers the post-upload feedback gap.

## Migration

No user action required.
