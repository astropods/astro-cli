# Summary

The blueprint setup support footer pointed builders to the old Slack invite. This updates the draft setup guide to send users to the current Astro Discord community instead, addressing issue #1181.

# Design

The draft blueprint setup footer keeps the same support layout and documentation CTA, but the community CTA now uses an outline Discord mark as a `currentColor` SVG so it inherits the outline button foreground in both themes and visually matches the docs CTA. Discord remains in the integration icon catalog for branded integration badges, and the reusable social icon is covered in Storybook. The component test covers the visible label and destination so future CTA changes are intentional.

# Migration

No migration required.
