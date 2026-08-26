# Every account gets a WorkOS organization

## Summary

WorkOS scopes applications, roles, and groups to an organization. Until now, only organization accounts had one. Personal accounts had none, so they could not own any of those things.

OAuth apps hit this first. Creating an app needs an `organization_id`, so `POST /accounts/:account/apps` refused every personal account.

The real rule is that a personal account holds one person. The account type sets that, not the presence of a WorkOS organization. This change gives every account an organization and moves the one-person rule onto the type.

## Design

### What each check means

| Check | Meaning | Can it change? |
| --- | --- | --- |
| `Type == "organization"` | The account may hold more than one person | No |
| `WorkOSOrganizationID != ""` | The organization link exists | Yes. Empty only until provisioning finishes |
| `session.OrganizationID == acct.WorkOSOrganizationID` | This caller's JWT claims apply to this account | Per request. Authorization reads this first |

Four call sites read the link but meant the type: invitations, member roles in the member list, the alert manager lookup, and the Insights role fallback. They now check the type. So a personal account still cannot be invited into, even though it has an organization. `org.Sync` already checked the type when adding, removing, and re-roling members.

Where a path still reads the link, it needs the ID to call WorkOS. A missing link on an organization account is a broken state, not a normal one, so those paths say so instead of skipping quietly:

| Path | Missing link on an organization account |
| --- | --- |
| Create or list invitations | `409`, logged as an error |
| OAuth apps | `409`, so the caller can retry |
| Alert manager lookup | Returns an error. The deliverer logs it and still alerts the owner |
| `org.Sync` role and member changes | Already returned an error before this change |
| Insights role fallback | Returns false, which restricts the caller to their own rows |

Account creation rolls an organization account back when provisioning fails, so this state should not occur. The log line is there to prove that.

### Authorization: the session's scope decides, not the account type

```mermaid
flowchart TD
    Req["account-scoped request"] --> App{"caller is an app?"}
    App -->|yes| AppScope{"app bound to this account<br>and holds the scope?"}
    AppScope -->|no| F403["403"]
    AppScope -->|yes| Pass["proceed"]
    App -->|no| Scoped{"session.org == account.workos_org_id?"}
    Scoped -->|no| Switch["403 use switch-org first"]
    Scoped -->|yes| Perm{"permission in JWT?"}
    Perm -->|no| F403
    Perm -->|yes| Pass
```

The rule used to start with the account type. It now starts with the session. A JWT scoped to the account's organization answers from its permission claims, for a personal account as much as an organization one. There is no account-type branch left in it. `HasAccountPermission` holds the rule and the middleware calls it, so the two cannot drift apart.

The web app now scopes its session to the personal organization, so browser callers take this path. Login does it: when AuthKit returns no organization, the callback re-mints the session against the user's personal organization. That is one extra WorkOS refresh at login, and it means landing on your own account costs no round trip afterwards. A user with no personal account yet, or whose organization link is still pending, is left unscoped.

Two write paths already scoped the session on demand and excluded personal accounts, the deploy variable picker and blueprint publishing. They now scope for any account with an organization. The session refresh asks for the organization the session already records, so a refresh cannot quietly return claims for a different scope than the session reports.

A scoped session is answered by its claims alone, including when they refuse. Being the account owner grants nothing on top, and no code path lets an owner around a permission the JWT withholds.

One fallback sits under the diagram, for callers that send no scope at all: a personal account with an unscoped session is answered by membership, as before. That is version tolerance, not privilege. An installed CLI binary sends an unscoped token, a browser session sealed before this change stays unscoped until the next login, and a session scoped to another organization is unscoped as far as this account is concerned.

`astro-cli` 0.17.1 mints the org-scoped token for every account, so a current CLI takes the JWT path too. It keeps the stored personal token only for the calls that have no account to scope to: device login, `/me`, and creating the account. A profile cached before that release carries no `workos_org_id` for the personal account, so it stays on the unscoped path until the next login refreshes the profile.

Once old binaries have drained, change what the fallback triggers on. Today it is caller-controlled: send no scope, get membership. Make it server state instead, so nobody can arrange it:

| Condition | Answer |
| --- | --- |
| Account has an organization | The JWT decides, always |
| Account has no organization yet | A personal account falls back to membership |

The second row happens only while provisioning is pending, where there is no permission set to bypass. Keeping that narrow case means a WorkOS outage during signup does not lock someone out of a brand-new account.

An organization account never falls back to membership. That direction would let any member pass every permission check by being a member, so a `member` would hold `org:manage`.

Because the claims now refuse, the WorkOS `admin` role has to carry every permission an account-scoped route asks for. The full set is small:

| Permission | Asked for by |
| --- | --- |
| `org:manage` | Account settings, members, invitations, apps, billing, audit log, Insights for everyone |
| `agents:write` | Agent visibility |
| `variable:read`, `variable:write` | Account variables and secrets |

All four are on the `admin` role, so a scoped session answers every account-scoped route. A permission removed from that role later is a 403 on the owner's own account, so the role set is now part of the contract.

### What is left to finish it

| Piece | State |
| --- | --- |
| Server: authorize from the JWT whenever the session is scoped | Done |
| Web: scope the session to the personal organization at login | Done |
| Verify `org:manage`, `agents:write`, `variable:read`, and `variable:write` on the WorkOS `admin` role | Done. The role carries all four |
| CLI: mint the org-scoped token for personal accounts too | Done in the `astro-cli` submodule, released as `0.17.1`. This repo's pointer moves when that merges |
| Remove the last membership fallback, once old CLI binaries have drained | Not started |

The payoff is larger than one branch. Personal accounts are excluded from FGA resource management, access groups, deployment visibility, the access service, and the Insights role fallback, in each case only because they had no organization. The cost is that personal-account authorization starts depending on WorkOS claims, where today membership lives in our own database.

### Provisioning

`org.Provisioner.EnsureOrganization(accountID)` does three things:

1. Find or create the WorkOS organization.
2. Save the link in `account_organizations`.
3. Give the owner an `admin` membership. Ownership itself is `accounts.owner_user_id`; WorkOS has no owner role.

```mermaid
flowchart TD
    Start["EnsureOrganization(accountID)"] --> Load["accounts.GetByID"]
    Load --> Linked{"link saved?"}
    Linked -->|yes| Own["ensureOwnerMembership"]
    Linked -->|no| Lookup["GetOrganizationByExternalID(accountID)"]
    Lookup -->|found| Adopt["reuse it"]
    Lookup -->|not found| Create["CreateOrganization(name=handle, external_id=accountID)"]
    Adopt --> Link["SetWorkOSOrganizationID"]
    Create --> Link
    Link --> Own
    Own --> Has{"owner has a membership?"}
    Has -->|no| Mint["CreateMembership(role=admin)"]
    Mint --> Record["UpsertMemberByWorkosMembershipID"]
    Has -->|yes| Record
    Record --> Done["return organization ID"]
```

You can call this as many times as you like. That matters because steps 1 and 2 write to two different systems, and no transaction covers both. A crash, a DB error, or a timeout can leave an organization in WorkOS with no link in the database. The account then looks unprovisioned, and the sweep picks it up again.

Each organization stores the account ID in its WorkOS `external_id`. So the next run looks up the organization by that ID first. If it finds one, it reuses it instead of creating a second one.

### Account creation

```mermaid
flowchart TD
    Post["POST /api/v1/accounts"] --> Row["insert account, owner member, profile"]
    Row --> Cfg{"WorkOS configured?"}
    Cfg -->|no| Created["201 Created"]
    Cfg -->|yes| Ensure["EnsureOrganization"]
    Ensure -->|linked| Created
    Ensure -->|error| Which{"account type"}
    Which -->|organization| Undo["DiscardOrganization<br>delete the account"]
    Undo --> Failed["500"]
    Which -->|personal| Enqueue["queue account.org_provision"]
    Enqueue --> Created
```

Both types now call the provisioner. They treat a failure differently:

- **Organization account:** the request fails and the account is deleted. Members get their permissions from the WorkOS JWT, so without an organization nobody could administer the account.
- **Personal account:** the request succeeds. The server logs the error and queues a retry. Personal accounts authorize by membership, so they work without the link.

### Backfill

```mermaid
sequenceDiagram
    participant River
    participant Sweep as account.org_provision_sweep
    participant DB
    participant Job as account.org_provision
    participant WorkOS

    River->>Sweep: hourly, and on worker start
    Sweep->>DB: accounts with no link, limit 200
    DB-->>Sweep: pending accounts
    loop per account
        Sweep->>Job: queue, deduped by account ID
    end
    Job->>WorkOS: look up by external_id, create if missing
    Job->>WorkOS: create the owner membership if missing
    Job->>DB: save the link and the membership ID
```

Two River jobs do the work:

- `account.org_provision` provisions one account, deduped by account ID.
- `account.org_provision_sweep` runs hourly and on worker start. It queues up to 200 unlinked accounts per run.

The sweep looks for a missing row:

```sql
FROM accounts a
LEFT JOIN account_organizations ao ON ao.account_id = a.id
WHERE ao.account_id IS NULL AND a.deleted_at IS NULL
```

There is no "provisioned" flag. The missing row is the only signal, so one query covers old accounts, dropped signup retries, and accounts that lose the link later.

`account.org_provision` leaves `completed` out of its River unique states. Otherwise a finished job would stop the next sweep from queueing the same account again.

Both jobs run only when WorkOS is configured. Account deletion already deleted the linked organization, so that path is unchanged.

### OAuth apps

The apps routes were always account-scoped. Only the handler blocked personal accounts. It now checks for the link, and returns `409` when it is missing so the caller can retry. The panel moved to `AppsSettings.tsx`, and both the personal and organization settings pages render it.

## Migration

Nothing to do. The sweep starts with the worker and runs hourly, 200 accounts at a time.

Until an account is linked, its OAuth apps page says the account is still being set up.

Watch WorkOS rate limits during the first sweep. Each account costs one lookup, up to one organization create, and up to one membership create.
