**Summary**

Polishes Slack identities in Insights so unlinked Slack users are easier to understand without making their names visually secondary to Astro members.

**Design**

- Slack profile labels render at the same table size as Astro member names.
- Slack profile labels carry the unlinked-member tooltip directly on the Slack name, including enriched names such as `rodric.rabbah`.
- Used-by avatars restore hover tooltips and include username context when available.
- The Insights table header keeps the Slack connect/resync action inline, and only shows it when Slack rows are missing profile details.
- Slack identities are visually dimmed consistently in both the People tab and Agents Used by column to distinguish non-Astro-profile identities while keeping row typography consistent.
- System-spend rows use the same dimmed treatment as non-profile identities.
- Rank numbers in the first column are left-aligned with the Name header to make the table scan more predictably.

**Migration**

No migration required.
