# Personal-account agents first in the agent switcher

## Summary

The agent switcher (the dropdown on the agent detail and chat headers) listed account groups in whatever order the deployments summary returned them. When you belong to one or more organizations, your own personal agents could sit below the org groups, so reaching an agent you deployed yourself meant scrolling past everyone else's. Your personal account now always comes first.

## Design

The switcher derives its groups from the deployments summary. After the existing eligibility filter, the account list is ordered the personal account first, then the remaining organizations alphabetically by name.

That ordering is shared with the top-level org switcher through one helper, so there is a single place to change the rule:

```ts
export function comparePersonalFirst(a, b) {
  if (a.type !== b.type) {
    if (a.type === "personal") return -1;
    if (b.type === "personal") return 1;
  }
  return a.name.localeCompare(b.name);
}
```

Both `OrgSwitcher` and `AgentDeploymentMenu` sort with `comparePersonalFirst`. No new data is fetched: the account `type` is already part of the summary payload.

## Migration

None. The change is presentation-only and takes effect the next time the switcher opens.
