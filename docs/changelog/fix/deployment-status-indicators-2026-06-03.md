## Summary

The agent deployment status surfaces disagreed with each other and one of
them was unreadable in light mode. Specifically (per #1259):

1. The top-right `AgentStatusToggle` showed "Resuming" while the
   deployment-history `DeploymentTile` showed "Deploying" for the same
   transitional state.
2. The "Deploying" badge in `DeploymentTile` was yellow text on a pale
   yellow tint against a white card surface — effectively unreadable.

This PR resolves both. The third part of the issue (image-pull failure
detection) is intentionally out of scope and will be tackled separately.

## Design

**One vocabulary for the transition.** The toggle and the tile now use
the same label ("Deploying") whenever the deployment is coming up. The
toggle keeps its own simple intent-driven state machine — no shared
store, no refactor of the mutation layer — but its copy aligns with the
tile so operators stop seeing two different words for the same moment.
The paused-state tooltip becomes "Redeploy this agent" to match.

**Yellow accent during deploy.** While the toggle reads "Deploying", the
spinner + label render with the warning palette
(`text-yellow-700 dark:text-yellow-400`) rather than green. Active still
goes green; pausing stays stone. This brings the toggle in line with
`StatusBadge` and `DeploymentTile`, which already use the warning
palette for the same lifecycle state.

**Semantic tokens for `DeploymentTile`.** Status colours previously
referenced raw palette utilities (`--color-yellow-300`/`-400`,
`--color-green-600`, `--color-coral-600`). These don't theme-flip, which
is why light mode broke. They're now the semantic `--warning`,
`--success`, and `--error` tokens — the same ones `StatusBadge` uses —
which resolve to the `*-600` shade in light mode and `*-400` in dark
mode, staying readable on both surfaces.

```ts
// before
deploying: { badgeText: "var(--color-yellow-300)", ... }
// after
deploying: { badgeText: "var(--warning)", ... }
```

**Tooltip falls back when the server returns no details.** The status
endpoint introduced in #1258 always sets `details` to a string, sometimes
empty. The toggle used `statusDetails ?? fallback`, which only triggers
on null/undefined — empty strings silently replaced the static
"Pause this agent" / "Redeploy this agent" copy. Switched to `||` so
empty `details` falls back to the static label.

## Migration

No action required.
