# Push visibility from spec

## Summary

`ast push` previously prompted the user to choose public or private visibility on the first push, and asked for confirmation on subsequent pushes when the spec visibility differed from the server. This was unnecessary friction — the `meta.visibility` field in `astropods.yml` is the right place to declare intent.

## Design

The visibility prompt and confirmation dialog have been removed from the push flow. Visibility is now resolved entirely from the spec:

- `meta.visibility: public` → pushed as public
- `meta.visibility: private` → pushed as private
- Not set → defaults to private

The `promptVisibility`, `confirmVisibilityChange`, and `getAgentFromServer` helper functions are retained in the codebase for potential future use but are no longer called.

## Migration

No action required. Agents that were previously pushed as public will remain public on the server — visibility is only updated on the next push. To keep an agent public, ensure `meta.visibility: public` is set in `astropods.yml`.
