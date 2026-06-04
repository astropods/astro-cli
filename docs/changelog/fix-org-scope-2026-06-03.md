# Fix org scope in blueprint wizard

## Summary

Creating a blueprint scoped to an organization account failed with "session is not scoped to this organization, use switch-org first." The app's org switcher updates only a UI cookie — it never re-scopes the JWT — so write endpoints that check org permissions would 403 even after switching to an org in the nav.

## Design

Two changes to the blueprint wizard:

**Default org from the active account.** `selectedOrg` now initializes to `activeAccount` from `ActiveAccountProvider` rather than always defaulting to the personal account. If you're browsing as an org when you open the wizard, that org is pre-selected.

**JWT scope switch at publish time.** `handlePublish` scopes the JWT to the target org immediately before the create call, only if the session isn't already scoped there:

```ts
const acct = accounts.find(a => a.name === selectedOrg);
if (acct?.type === "organization" && acct.organization_id && acct.organization_id !== organizationId) {
  await switchOrg(acct.organization_id);
}
```

The check short-circuits when the session is already scoped correctly, and is skipped entirely for personal accounts. Deferring the scope switch to publish time (rather than on dropdown change) avoids a background network call on every org selection and keeps the UI synchronous.

## Migration

No migration required.
