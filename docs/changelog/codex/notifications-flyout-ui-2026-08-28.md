# Notification inbox interface

## Summary

The notification inbox now presents unread state, workflow context, and actions
more clearly across desktop and mobile layouts.

## Design

The desktop inbox remains a compact popover. Mobile layouts use the shared
bottom sheet so notifications have enough space for content and touch actions.
Both layouts share the same header, tabs, unread count, and notification rows.
The shared tab button lives in the components layer and exposes compact bottom
padding for constrained surfaces such as the inbox.

Notification workflow identifiers map to a small semantic icon set. The welcome
notification retains a distinct celebration icon. Semantic color tokens convey
informational, warning, critical, and success states. A consistent unread marker
appears on the notification icon. A pure resolver owns workflow lookup and the
neutral fallback so icon behavior can be verified without mounting Novu.
A text equivalent adds unread state to each row's accessible name.

Selecting a row marks the notification as read and follows its configured link.
Notification CTAs share activation logic across pointer, Enter, and Space input.
Internal notification links close the desktop popover or mobile sheet after
navigation. External links keep the inbox open while they open in a new tab.
The row action and its CTAs are separate interactive controls with a predictable
keyboard order and accessible names.
Desktop archive actions replace the timestamp on hover or keyboard focus. Mobile
archive actions remain visible. Per-row read toggles are omitted because row
selection marks a notification as read and the header supports bulk updates.
This keeps Archive as the focused row-management action. Timestamps use the
faint foreground token to stay subordinate to notification copy. Header icon
buttons mark all inbox notifications as read and open notification settings.
Shared tooltips label every icon-only action. Accessible names remain available
to assistive technology. The mobile header includes a visible close action.
The mark-all action stays visible. It becomes disabled when the current view
has no unread notifications.

## Migration

No migration is required.
