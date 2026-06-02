# Blueprint sidebar: rename Authors section to Contributors

## Summary

The blueprint detail sidebar labeled the people credited on an agent as "Authors". "Contributors" better matches how the section is used in practice — it includes commit authors, maintainers, and others who shipped the agent, not just the original author. Renamed the user-facing label only.

## Design

One-line copy change in `SidebarAuthor.tsx`: the `SidebarSection` title becomes `"Contributors"`. Internal types and props (`BlueprintAuthor`, `authors`, `SidebarAuthor`) are unchanged — they describe the data shape, not the UI label, and renaming them would churn unrelated call sites for no user-visible benefit. The unit test that asserts on the section title and the smoke test description in `public.blueprint.spec.ts` are updated to match.

## Migration

No action required.
