# Rename and publish astro-collector to Docker Hub

## Summary

`astro dev` was failing for all agents with a Docker pull error because the local observability collector image (`astropods/prod-astro-collector`) was never published to Docker Hub under that name. The image existed in ECR and GHCR but was not publicly available, blocking any user from running `astro dev` without registry credentials.

## Design

The image is renamed to `astropods/collector` across the board — matching the naming convention used by `astropods/messaging` and `astropods/playground`.

The CLI's compose builder hardcodes the collector image injected into every agent's dev environment. This is now `astropods/collector:latest`. The local Moon build task is updated to tag the image consistently so `deployment:collector` produces an image that matches what `astro dev` pulls.

Publishing follows the same multi-arch digest pattern used by the `playground` and `messaging` submodule workflows: each platform builds independently and pushes by digest, then a merge job creates the final multi-arch manifest. The existing `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` secrets are reused — no new secrets required.

## Migration

Nothing required. Users running `astro dev` will automatically pull `astropods/collector:latest` from Docker Hub after this change is merged and the workflow is triggered.
