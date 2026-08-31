# Spend-limit increase requests

## Summary

A spend limit stopped at $1,000, the self-serve ceiling, and the only route past
it was an email. The account typed a higher number, the save failed, and a toast
told it to write to support. Nothing recorded the ask, nothing put it in a queue
an admin already reads, and raising the ceiling afterwards was a manual write to
a table.

The account now files a spend-limit increase request from the field it was
refused in. The request travels the same pipeline as a resource quota request:
the same table, the same admin list in Queen, the same approve and deny buttons.
Approving raises the ceiling the account may set its own limit to.

## Design

### One requestable key that is not a resource

Quota gates resource *counts* and billing gates *metered consumption*, and the
boundary between them is deliberate. A spend limit belongs to billing: it has no
rows to count, and the billing provider enforces it. Making it a seventh
`quota.AllResources` entry would have put a key with no count query in front of
the checker and the usage report.

So the vocabulary splits instead. `quota.IsResource` is unchanged, and a wider
predicate governs what may be *asked for*:

```go
const KeySpendLimit = "spend_limit"

func IsRequestable(key string) bool {
    return IsResource(key) || key == KeySpendLimit
}
```

The request handler and `ApproveQuotaIncrease` moved to `IsRequestable`. Every
other quota path still reads `IsResource`, so the checker never sees the key and
the usage meters never render it.

### A grant is a ceiling, not a limit

The approval writes `account_limits` with `resource = 'spend_limit'` and the
grant in whole dollars, through the existing transaction. What that row holds is
the **highest limit the account may choose**, not the limit itself. Approving
raises what is allowed; the account still picks a number under it, so no
approval charges anyone more.

`quota.SpendCeilingUSD` resolves it: a grant when one exists, else
`billing.MaxSelfServeSpendUSD`. A grant never lowers the ceiling, which is what
makes granting too little unable to cut an account below the default, and makes
the `-1`/`0` sentinels resource limits carry read as the default here rather
than as no spend at all.

Two readers, and a grant only half-lands without both:

- `SetBillingSpendThresholds` bounds what the account may write. It used to
  compare against a package constant.
- `BillingGatewayBudgetWorker.ceilingUSD` clamps the AI gateway budget.
  Clamping to the shared default here would leave the gateway refusing spend
  the billing provider had already accepted, which is the grant not landing.

`GET /billing/spend` gained `spend_ceiling` in the same minor units as the
thresholds beside it. The dialog needs the account's own number, or it keeps
refusing a figure the server now accepts and sends a second request for a
ceiling that is already granted.

### Routed from the field, not from the quota page

The request form opens from the spend-limit field that refused the number, not
from the quota section in Settings. The feature picker there offers what the
usage endpoint meters, and the spend limit is not one of those; more to the
point, the account is already looking at the control it wants raised.

`ManageLimitsDialog` blocks a limit above the ceiling with the ceiling named and
a "Request an increase" action, then swaps itself for `RequestIncreaseDialog`
under the `spend_limit` key rather than stacking a second dialog over the fields
the request is about.

The request form learns one mode from `meterMeta`. A `money: true` key renders
every amount as currency and makes the requested amount required, because a
reviewer cannot read a spend figure off a count the way a resource request lets
them. The server enforces the same two rules: an amount is required, and it must
exceed the current ceiling, since an amount at or under it is already the
account's to set without asking.

Queen labels the row `Spend Limit ($/mo)` and formats its amounts as currency,
so an operator approving one is not reading dollars as a count.

### The operator path to the same numbers

A request-and-review loop is the customer's route. An operator needs one that
does not wait on it, so the Billing section of the Queen account page sets the
account's spend limit directly, at any value, whatever the account's billing
state. `MaxSelfServeSpendUSD` governs what a customer may choose for itself; it
was never meant to govern a grant.

`SetAccountSpendLimit` does one write across both numbers: the limit at the
provider, then the ceiling raised to match, then a gateway budget re-derive.
Leaving them apart is the same half-landing the clamp above describes, reached
from the other side. Clearing the limit leaves the ceiling: a ceiling is a
grant, and dropping a limit is not revoking it.

The control disables only when the account has no Metronome customer, because
there is then nothing to write a limit to. Suspension, dunning, and missing
provisioning all leave it usable, which is the point.

## Migration

None. No schema change: both `quota_increase_requests.feature_key` and
`account_limits.resource` are free-text columns already. An account with no
approved request keeps the $1,000 ceiling it had.
