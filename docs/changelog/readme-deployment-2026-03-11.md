# Add deployment documentation to README

## Summary

Added a Deployments section to the README explaining the preview and production deployment workflows so contributors know how changes reach each environment.

## Design

- Preview deploys automatically on merge to `main`.
- Production deploys are triggered manually via the "Deploy (Prod)" GitHub Actions workflow, selecting which services to deploy (`astro-server`, `astro-client`, `astro-registry`). CLI releases use a separate "Release CLI (Prod)" workflow.

## Migration

No migration required.
