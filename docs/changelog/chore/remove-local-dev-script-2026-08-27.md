# Remove scripts/local-dev.sh

## Summary

`scripts/local-dev.sh` wrapped the two moon dev targets, a Traefik container,
and the `ast-dev` CLI build into one command. It is gone. Running the platform
locally is now the moon targets directly, with Traefik as a separate opt-in
step.

## Design

The script offered one thing the targets do not: a single origin at
`http://localhost`, with Traefik routing `/api` to the server and everything
else to the client. That capability stays. `docker-compose.local.yml` and the
`traefik/` config are untouched, so a single-origin setup is one command:

```bash
docker compose -f docker-compose.local.yml up -d
```

The README documented two ways to run and now documents one, with Traefik as an
addition to it rather than a second path. Smoke tests default to
`ASTRO_ENV=dev` against `http://localhost`, so their instructions now name
Traefik explicitly instead of naming the script that used to start it.

The `ast-dev` build the script performed is already covered by
`moon run astro-cli:link`, which the README describes in the agent development
section.

References removed from the repo README, `scripts/README.md`, and
`apps/astro-client/README.md`. Release notes mentioning the script are
historical records and stay as they are.

## Migration

Replace `./scripts/local-dev.sh` with two terminals:

```bash
moon run astro-server:dev
moon run astro-client:dev
```

Add `docker compose -f docker-compose.local.yml up -d` if you want everything
on `http://localhost`, which smoke tests require.
