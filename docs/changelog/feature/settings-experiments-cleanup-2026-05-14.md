## Summary

The `theming` experiment has graduated out of the experiments system and is now a first-class feature. This removes the experiment gate, cleans up the now-empty experiments tab from the settings nav, and makes the theme switcher in the profile dropdown always visible.

## Design

`experiments.ts` now exports `hasExperiments` (derived from `Object.keys(DEFAULTS).length > 0`), which `SettingsLayout` uses to conditionally render the Experiments nav item. When no experiments are defined the tab disappears automatically — no manual gating needed when future experiments are added. Navigating directly to `/settings/experiments` redirects to `/settings/account` when the page has no content.

The `ThemeSwitcher` (three-button light/dark/system picker) that previously lived in the profile dropdown behind the `experiments.theming` flag is now rendered unconditionally — same location, no flag required. The `SectionHeader` component also had a stray `flex-1` class removed, which was causing an oversized gap in settings pages that only rendered a header with no content below.

The `theme.spec.ts` e2e suite is updated to match: tests now assert the switcher is always present in the dropdown without any experiment setup.

## Migration

No action required. The `astro:theme` localStorage key is unchanged. Any previously stored theme preference continues to work.
