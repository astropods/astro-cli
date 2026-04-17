## Summary

The acronym capitalization fix applied only to boolean toggle labels; the primary `<Label>` elements above text and secret inputs used a separate, unfixed copy of the humanize function in `VariableFields.tsx`. Also fixes a flaky e2e test in the GitHub onboarding suite.

## Design

`VariableField.tsx` and `VariableFields.tsx` each had their own `humanizeKey` function, but only the one in `VariableField.tsx` included the acronym post-pass. The duplicate in `VariableFields.tsx` (which renders the `<Label>` above every non-boolean field) is removed; `VariableFields.tsx` now imports `humanizeKey` from `VariableField.tsx`. The function is also renamed from `labelFromKey` to `humanizeKey` for consistency. All label rendering paths (boolean toggles, text inputs, secret inputs, selects) now go through the same function. 16 `DeployBlueprint.test.tsx` expectations updated to match the corrected output (e.g. `'Openai Api Key'` → `'OpenAI API Key'`).

The flaky `github-onboarding.spec.ts` e2e test was asserting blueprint card visibility immediately after `domcontentloaded`, before the sequential account + agents API fetches had resolved. Fixed by registering a `page.waitForResponse` for the blueprint list response before navigating, matching the same pattern used for the archive response later in the same test.

## Migration

No action required.
