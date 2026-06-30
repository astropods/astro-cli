## Summary

The personal account settings still showed the username as a read-only input with a pencil-icon affordance, while the organization settings had already moved to a cleaner treatment: a static `@handle` with a "Change username" link. This brings the personal account section in line with the org redesign and fixes a set of mobile layout problems across the settings pages.

## Design

The personal username field now renders the same way as the org variant — a static mono `@handle` followed by a `Button variant="link"` that opens the existing `ChangeUsernameDialog`. There is no read-only/permission branching here because a user always owns their own personal account, so the disabled-with-tooltip path that the org section needs is omitted.

Mobile robustness was the second theme. Three rows that previously assumed they always had room are now allowed to reflow:

- The username rows (personal and org) use `flex-wrap` with split `gap-x-3 gap-y-1.5`, so the "Change username" link drops snugly below the handle on narrow widths instead of either overflowing or inheriting the full 12px gap as vertical space.
- The display-name row wraps and gives the input a `min-w` while the save button is `shrink-0`, so the button moves to its own line rather than being compressed against the input.
- `SectionHeader` stacks its action beneath the title/subtitle below the `sm` breakpoint and only goes side-by-side from `sm` up, fixing the action button overlapping the title (e.g. "Create organization" over "Organizations").

The `SectionHeader` change is shared, so every settings page that passes an `action` benefits.

## Migration

No action required.
