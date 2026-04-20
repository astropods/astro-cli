# Strict spec validation on `ast push`

## Summary

`ast push` previously only ran the semantic parser on `astropods.yml`, skipping the JSON-schema checks that `ast validate` performs. Users could push specs that were missing required fields (e.g. `meta`) or otherwise failed validation — the build and image upload would run, and the server would either reject the spec after the fact or accept a spec inconsistent with the schema. This tightens push so an invalid spec fails fast, before any docker build, registry upload, or server registration.

## Design

The validation core from the `validate` command (YAML parse + JSON schema + semantic rules, with line-annotated error output) is now exposed as a shared helper and called as the first step of `push`. On any validation error, push prints the same diagnostics as `ast validate` and exits non-zero; no build or push is attempted.

- `validateSpecFile(specPath)` returns the parsed `*spec.AstroSpec` on success so `push` does not re-parse.
- `runValidate` delegates to the same helper; its output format is unchanged.
- `runPush` replaces the old `Parsing …` step with `Validating …`, calling the helper before auth, build, and any image operations.

## Migration

None. Specs that already pass `ast validate` will push unchanged. Specs that did not pass `ast validate` but were being pushed regardless will now fail at `push` with the same errors `ast validate` reports — fix the errors (or run `ast validate` locally first) and re-push.
