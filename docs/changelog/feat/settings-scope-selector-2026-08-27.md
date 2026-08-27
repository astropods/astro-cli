# Settings scope selector and grouped nav

## Summary

Settings had two disconnected shells. Personal settings lived at `/settings/*`
with a flat 11-item sidebar; org settings lived at `/settings/org/:slug/*`
with its own flat sidebar, its own big org-name heading, and a "← Settings"
back link. The only route into org settings was the Organizations list, so
moving between scopes meant walking back out to that list and clicking in
again, losing whatever section you were on.

This replaces both sidebars with one: a scope selector on top, and nav items
grouped into Manage, Access, and Integrations.

## Design

**One sidebar, two scopes.** `components/settings/SettingsSidebar.tsx` renders
the heading, the selector, and whatever grouped nav its shell passes as
children. `SettingsLayout` and `OrgSettingsLayout` stay separate route trees
(they always were), they just share the chrome now.

**The selector navigates, it does not switch the app.** Picking an account
routes to the same section in the target scope through `settingsScopePath`:
shared sections carry over (`billing` → `billing`), the personal Account page
and the org General route stand in for each other, and anything with no
counterpart (Connectors, Organizations, Members) lands on that scope's first
page. The current section is read back off the URL, so the selector holds no
state.

It deliberately doesn't call `setActiveAccount`. Settings scope is URL-derived
and `OrgSettingsLayout` already owns the WorkOS `switchOrg` for the org in the
URL; wiring the selector into the app-wide active account would move the
agents list and deploy targets as a side effect of opening a settings page.

**Groups that survive the mobile collapse.** `SidebarNav` renders its children
three times (a pill row, a dropdown, and the desktop column) and picks one
with CSS. A group wrapper would have broken the pill row, so `SidebarNavGroup`
is `display: contents` below `md` and only shows its label on desktop. The
active-item lookup that feeds the mobile dropdown trigger now recurses instead
of scanning direct children.

**Unbuilt sections say so.** Org Connectors renders through
`SidebarNavPlaceholder` as a non-navigable row tagged "Coming soon" rather
than being silently absent. Groups (designed for both scopes) is not in the
nav at all: the server has access-group CRUD but the client has no page, and a
nav item pointing at nothing is worse than an omission.

**Organizations page.** Rows no longer link into org settings, since the
selector is now that path. Each row gets a Leave button wired to the existing
`LeaveOrganizationDialog`.

Nav items lost their icons and org "General" is now labelled "Account", so the
two scopes read as the same menu. Item states follow the design: foreground
text at regular weight, medium weight plus a `bg-secondary` fill when active.

While in `OrgSettingsLayout`, its 403 and switch-failed panels moved off
hand-rolled `stone-*` buttons onto `Button` and semantic tokens, dropping the
`no-raw-theme-colors` baseline from 60 to 51.

## Migration

None. No routes changed, no API changed.
