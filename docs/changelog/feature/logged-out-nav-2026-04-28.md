## Summary

The logged-out navigation lacked visual hierarchy, consistent treatment across elements, and a coherent responsive strategy. "Sign up" was generic, button sizes were mismatched, and the mobile nav buried auth actions behind a hamburger menu.

## Design

### Desktop

Reorganized the right-side nav into two clear zones: secondary navigation links (Explore, Docs, Blog) and auth actions (Log in, Get started).

- Explore moves into the nav link group as a plain text link with icon — matching the weight and color of Docs and Blog
- "Sign up" renamed to "Get started", sized down to `sm`
- Log in rendered as `ghost` / `sm` — softer than the primary CTA, heavier than a plain link
- Auth button pair uses `gap-2`; outer nav group uses `gap-4`

### Mobile

All secondary links (Explore, Docs, Blog, Feedback) move as a group: visible in the header at 700px+, in the hamburger sheet below that. The hamburger is hidden entirely when logged out at 700px+ since there's nothing left in the sheet.

- Log in and Get started are always exposed in the mobile header — no longer buried in a sheet
- Log in moves to the hamburger below 380px to prevent overlap with the logo
- Explore has no icon when in the sheet; Log in matches the same sheet link style
- Logged-in mobile follows the same rules with the same breakpoints

No migration required.
