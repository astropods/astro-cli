# Preset User Avatars

## Summary

Users had no profile picture by default, falling back to a plain initials circle. This adds a set of 25 pixel art preset avatars that are deterministically assigned based on a stable seed string (user ID, account handle, or author name), so every user has a consistent, distinct avatar without any backend changes or user action required.

## Design

A `PRESET_AVATARS` constant in `src/lib/presetAvatars.ts` holds the typed asset references for all 25 PNGs. A `getPresetAvatar(seed)` utility hashes the seed string to consistently select an avatar from the set.

Three components were updated to use this fallback:

- **`UserAvatar`** — hashes `user.id`; renders as `rounded-lg` (square) rather than `rounded-full` to suit the pixel art format
- **`SidebarAuthor`** — hashes `author.account ?? author.name` for agent card authors, `ownerHandle` for the account owner fallback
- **`AccountProfile`** — hashes `data.name` (account handle) for the profile header

A `PresetAvatarPicker` component is also included for future use when users can manually select their avatar. It renders a 5-column grid with radio semantics and a selection ring. The save action is stubbed pending a backend `preset_avatar_id` field.

## Migration

No migration required. Existing users without a `profile_picture_url` will automatically receive a deterministic preset avatar based on their user ID. The assignment is consistent across sessions and devices.
