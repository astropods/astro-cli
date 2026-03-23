## Summary

This update standardizes deployment and monitor status messaging around a shared panel composite system so all user-facing banners are visually and behaviorally consistent. It also removes direct hardcoded palette usage from these panels in favor of design-system color tokens.

## Design

The deploy panel component was expanded into a tone-based composite family (`InfoPanel`, `SuccessPanel`, `WarningPanel`, `ErrorPanel`) built on a single base renderer. Each tone now derives text, background, and border values from design-system token variables, and all variants support a shared dismissible behavior and a new inline presentation mode for single-line notices.

Deployment-history and monitor notices now consume these shared composites instead of maintaining ad hoc banner styles. Monitor trace status badges were also moved to token-driven styling, and `InlineBadge` was extended to accept explicit style overrides so badge rendering can remain reusable while still honoring tokenized color inputs.

Storybook composites were updated to cover each panel tone with default and inline states, plus dismissible behavior where relevant, so UI behavior and token styling are easy to verify in isolation.

## Migration

No migration steps are required for users. Existing call sites continue to work with default panel behavior, and optional props (`dismissible`, `onDismiss`, `variant`) can be adopted incrementally.
