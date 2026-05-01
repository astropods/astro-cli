# Blueprint card metadata: owner vs publisher

## Summary

Blueprint cards on the Explore page now show the owner account (e.g. "Postman") in the card footer, while cards on the Blueprints page show the person who pushed the spec. Previously both surfaces showed the owner account unconditionally.

## Design

`BlueprintCard` gains an optional `author?: BlueprintAuthor` prop. When provided it replaces the account name in the card footer (avatar + handle). `BlueprintListView` gains a `showAuthor` boolean that passes `publishers[0]` as `author` to each card — the Blueprints page sets this, Explore does not.

To make `publishers` available on list responses (it was previously only populated by the detail endpoint), `BulkDistinctActorsFor` was added to the audit store. It resolves all publisher actor IDs for a set of agents in one query rather than one query per agent. `ListAgents` and `ListAccountAgents` call this before building responses, grouped by account to avoid cross-account fan-out. A shared `map[string]*auth.User` cache prevents redundant WorkOS lookups when the same person has pushed multiple blueprints.

The detail page sidebar (`SidebarAuthor`) is unchanged — it already showed publishers with a fallback to the account owner when publisher data is absent.

## Migration

No action required.
