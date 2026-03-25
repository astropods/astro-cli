# Deployment Live Reveal and Share Flow

## Summary

Introduced a post-deploy "live reveal" experience that celebrates when a deployment becomes live, while preserving predictable navigation between Deployments and Monitor. The update also standardizes badge-sharing actions so users can share or export the deployed agent card directly from the reveal state.

## Design

- **Deployment transition experience:** Added a full-screen reveal overlay with confetti and the generated trading-card badge when a deployment transitions into live for a new deployment identity. This avoids replaying the reveal for simple status churn while still honoring real new deployment launches.
- **Navigation contract:** Kept background context on Deployments during reveal, and only navigate to Monitor on explicit user action (`View monitoring`). This separates celebratory state from observability workflow and prevents accidental tab jumps.
- **Share architecture:** Consolidated sharing into a single dropdown with network actions plus deterministic asset export (`PNG`/`SVG`) from the same badge source used in the reveal, so shared artifacts match on-screen visuals.
- **Social preview reliability:** Added a dedicated public share surface with Open Graph/Twitter metadata to improve external preview generation for network share links where platform composers do not reliably prefill rich content.
- **UI consistency pass:** Aligned icon/button treatments and spacing in the reveal/share controls so action hierarchy is visually consistent with existing Astro client patterns.

## Migration

No migration required.
