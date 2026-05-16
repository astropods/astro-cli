## Summary

Account name validation blocked plurals (e.g. `blueprints`) but not their singulars (e.g. `blueprint`), and blocked some singulars (e.g. `admin`) without their plurals (e.g. `admins`). Either form could be registered as an account slug, colliding with existing or future app routes.

## Design

A new `reservedVariants` map in `internal/account/variants.go` holds the missing halves of every singular/plural pair present in `reservedNames`. `CheckAccountNameRestricted` now checks both maps, blocking either form with the same error.

The variants are kept in a separate file so `reservedNames` remains the canonical, curated list of intentionally reserved paths, while `variants.go` is clearly derived from it.

## Migration

No action required. Existing accounts are unaffected; only new registrations are checked.
