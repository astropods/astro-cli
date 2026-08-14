# Blueprint account name

## Summary

The blueprint detail page showed only the blueprint name. Two blueprints with the same name in different accounts looked the same, and nothing on the page said which account owned the blueprint. The page title now reads `account / name`, in the style of a repository path.

## Design

**Title lockup.** The account is muted and links to the account profile. The blueprint name keeps the strong weight, so the name stays dominant. The account was already available to the header component, so no new data is needed.

**One back link in place of the trail.** The breadcrumb showed the account, which the title now shows. The trail is replaced by a single link that keeps the previous root logic: `Back to explore` when the visitor arrives from Explore, `Back to blueprints` otherwise, and public discovery for signed-out visitors.

**Overflow.** The account link truncates at 14, 24, or 36 characters as the viewport grows. The server caps account names at 39 characters, so wide screens rarely cut a handle. A `ResizeObserver` measures real truncation to decide when to show the full handle in a tooltip. A fixed character count cannot do this correctly, because the cap changes with the breakpoint, so the tooltip would appear when the text is fully visible or stay away when the text is cut. The blueprint name always shows in full and breaks mid-word when it must wrap.

**Blueprint cards keep the name alone.** A grid card gives the title about 200px, and the account with the name needs near twice that at real name lengths. Both halves truncate, which costs more than it gives. Instead, the card footer always shows the owning account. The footer showed the first publisher on the authenticated Blueprints page, which is a contributor and can differ from the owner, so a card could state no owner at all.

**Card footer truncation follows card width, not the viewport.** The card grid sets its column count from container queries, so a wider screen adds a column and makes each card narrower. The narrowest cards are in the three-column range, not on the smallest screen. Viewport breakpoints would therefore truncate the wrong cards. The deploy count holds its width, and the owner name takes the space that is left, capped so that a long handle keeps clear space from the deploy count.

## Migration

No action is required.

Contributors no longer show on blueprint cards, because the footer now shows the owning account. Contributors continue to show in the blueprint detail page sidebar.
