## Summary

Deployment service accordions showed tabs (Variables, Domains, Events) even when they had no data, making them appear clickable but non-functional. Only tabs with data should appear.

## Design

Tab visibility is now driven by a single `tabs` array defined alongside the `canShow*` flags:

```ts
const tabs: Tab[] = [
  { id: "vars",    label: "Variables", visible: canShowVars },
  { id: "domains", label: "Domains",   visible: canShowDomains },
  { id: "events",  label: "Events",    visible: canShowEvents },
];
```

`canShowVars` previously only excluded the collector container — it now also requires at least one variable to be present. The rendered tab bar filters to `tabs.filter(t => t.visible)`, and the `effectiveView` fallback resolves through the same array, eliminating repeated name checks.

`TabId` and `Tab` types are extracted at the module level so the state, fallback logic, and render all share one definition.

## Migration

No action required.
