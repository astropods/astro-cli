## Summary

Very long organization names could spill out of the organization settings page
and overlap nearby content. They could also be created longer than we can
comfortably show in smaller places like switchers, cards, and settings
sidebars.

This change keeps long names readable in settings while also preventing new
organization names from becoming too long to display well across the product.

## Design

Organization settings now handle long names in two predictable ways:

- The main settings sidebar shows the full organization name and wraps it with
  hyphens when it needs more than one line.
- Compact rows, such as account headers, shorten the visible text but keep the
  full name available on hover.

Organization display names are now capped at 39 characters, matching the
existing organization username limit. That keeps the two public organization
names aligned and avoids creating names that will always need to be cut off in
common UI surfaces.

The limits are expressed as shared constants instead of separate magic numbers:
account usernames are capped at 39 characters, personal display names stay at
64 characters, and organization display names have their own explicit
39-character cap. The client-side display-name helper returns the same
validation message for every organization editor, while the server applies the
same rule when creating an organization or updating an account display name.

The 39-character rule is shared by the organization creation screen, the
organization settings editor, and the organization profile edit sidebar. When
someone types a longer name, each form shows the same inline message under the
text field and blocks saving in the submit handler. Display names are required
for both personal and organization accounts, matching the server rule, while
each kind keeps its own length limit. The buttons stay clickable unless a
request is already in flight, so keyboard and assistive-technology users can
still reach the action and receive validation feedback. Organization creation
also shows a page-level submit error when the username is still being checked,
unavailable, or when an invitation entry needs attention, so clicking the button
never fails silently.
The server also checks the same limit when organizations are created and when
account display names are updated. If an organization update explicitly sends an
empty display name, the server rejects it; profile-only updates that omit
`display_name` keep working as PATCHes. Personal display names keep their
existing 64-character limit and now use the same shared error message path as
organization names.

Client and server validation both count display-name length as Unicode code
points, not stored bytes. That matters for names with accents, emoji, or other
non-ASCII characters: a name that visually fits the limit is accepted
consistently in the browser and by the Go API.

The organization settings editor and profile edit sidebar also share the same
Save button treatment. While the rename request is in flight, the button stays
disabled and shows the same spinner so people can see that the round trip is
still happening. Settings no longer shows a separate "Saved" checkmark after
the spinner clears, so the save state has a single source of truth. If a save
fails, the editor now keeps the draft in place and shows the server's error
message under the field when one is available. The error-message extraction is
shared with the organization creation flow so API errors are surfaced the same
way instead of being parsed separately in each form. The settings editor also
reserves space for validation text so the Save button and surrounding layout do
not jump when an error appears.

After the server confirms a display-name save, the app updates the signed-in
account list, account detail views, and organization membership lists in memory
before the background refresh finishes. This keeps settings, profile sidebars,
switchers, and tooltips in sync as soon as a saved organization name is
available.

The cache path is intentionally scoped to the places that currently render
organization display names: the authenticated account list used by switchers and
settings navigation, the account detail query used by profile/settings views,
and cached organization membership lists used by profile sidebars and their
tooltips. Those values are patched after a successful server response, then the
same queries are invalidated so the background refetch can reconcile any other
server-owned fields without reintroducing visible name lag. The cache patching
is restricted to exact organization-membership query keys so account-name
checks and search results cannot be mistaken for org membership lists.

## Migration

No immediate action is required. Existing organizations with longer display
names can still be viewed, but they must be renamed to 39 characters or fewer
the next time the display name is edited.
