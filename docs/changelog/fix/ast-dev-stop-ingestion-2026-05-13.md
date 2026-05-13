# ast dev stop now kills ingestion containers

## Summary

`ast dev stop` (and Ctrl+C from `ast dev` foreground) used to leave running ingestion containers behind. Users would see `docker ps` still showing a `<project>-ingestion-<name>-run-<slug>` container long after stopping the dev environment, with compose itself warning `Network ... Resource is still in use`.

## Design

Ingestions are launched via `compose run` (`RunOneOffContainer`), which tags containers with `com.docker.compose.oneoff=True`. `compose down` excludes one-off containers by default — it switches to including them only when `RemoveOrphans` is set. The CLI's stop paths called `Down` with empty options, so the project's services and network came down but the one-off ingestion containers survived.

Fix: pass `api.DownOptions{RemoveOrphans: true}` in both teardown paths in `apps/astro-cli/cmd/dev.go`:

- `runDevStop` — invoked by `ast dev stop`
- `runForeground` — the Ctrl+C cleanup for `ast dev` in non-background mode

The startup-time cleanup at the top of `runDevStart` already used `RemoveOrphans: true`, so cold starts were unaffected.

## Migration

None — behavior change is purely additive (more containers cleaned up on stop).
