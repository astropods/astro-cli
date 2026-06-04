## Summary

Renaming an account in Astro updated Postgres and WorkOS but left the linked OpenMeter customer display name unchanged. Queen’s Customers list reads `customer.name` from OpenMeter (set at customer creation), so renamed orgs could keep showing the old label (e.g. `tester` after rename to `eu-testing-org`).

## Design

**HTTP rename path only.** After a successful `PUT /api/v1/accounts/:account` rename, the handler loads `openmeter_customer_id` for the account and, when OpenMeter is configured and the name actually changed, calls OpenMeter `PUT /api/v1/customers/{id}` with `{ "name": "<new name>" }`.

**Warn-only sync.** OpenMeter lookup or update failures are logged and do not fail the rename — billing identity stays tied to the stable customer ID; display name drift is preferable to blocking account renames.

**Client surface.** New `openmeter.Client.UpdateCustomerName` and `account.AccountStore.GetOpenMeterCustomerID` support the handler. `main.go` passes the existing `omClient` into `RenameAccount`.

**Out of scope.** Admin gRPC `RenameAccount` is unchanged; only the user-facing HTTP rename is wired today.

## Migration

No action required for new renames — names stay in sync automatically when OpenMeter is enabled.

Accounts renamed before this change may still show the old name in Queen until the account is renamed again or the OpenMeter customer is updated manually.
