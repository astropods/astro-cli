# OpenMeter prod plan reference

## Summary

The OpenMeter integration doc previously documented only the preview environment's `private_beta` plan. The prod plan has diverged (different feature keys, limits, and reset cadence in places), so engineers debugging entitlement issues against prod had no canonical reference and were reading it out of the OpenMeter API.

## Design

Two changes to `docs/03-architecture/openmeter-integration.md`:

- The existing plan JSON block is now labeled **Preview** so it's unambiguous which environment it describes.
- A new **Prod** subsection follows it with the full plan definition as currently registered in prod OpenMeter (`POST /api/v1/plans` payload). Both blocks live under the same "Plans" heading so they can be diffed at a glance.

Markdown tables in the meters, CU/heartbeat, and entitlements sections were reformatted with aligned column widths. No content changes — just whitespace — to make future diffs to those tables readable.

Submodule bumps for `modules/playground` and `modules/website` are included as routine pointer advances tracking their upstream `main`.

## Migration

None. Documentation-only.
