# Fix moon run astro-server:dev orphaning child processes

## Summary

Ctrl+C on `moon run astro-server:dev` was leaving the compiled `astro-server` binary (spawned by `air`) and the compiled `fakeopenmeter` binary (spawned by `go run`) running. Both got re-parented to launchd with `PPID=1` and kept their listeners on `:8080` and `:8888`. The next `moon run astro-server:dev` then died on bind:

```
00:18:25 INFO  Server listening address=0.0.0.0:8080
00:18:25 ERROR Failed to start server error=listen tcp 0.0.0.0:8080: bind: address already in use
Process Exit with Code: 1
```

The dev script already had a `trap cleanup EXIT INT TERM`, but it `kill`ed only the direct child PIDs (`air`, `go run`). Neither parent reliably forwards SIGTERM to its compiled grandchild, so the grandchildren survived.

## Design

`scripts/dev.sh` now enables bash job control (`set -m`) so each `&`-backgrounded child becomes the leader of a new process group. Cleanup targets the *group* instead of just the leader:

```bash
kill -- "-$SERVER_PID"      # kills air + tmp/astro-server in one shot
kill -- "-$FAKEMETER_PID"   # kills go run + the compiled fakeopenmeter
```

The trap is also guarded with `_cleanup_ran` so the shutdown banner only prints once even though the trap fires for both INT and EXIT.

## Migration

None. Developers running `moon run astro-server:dev` get the new behavior automatically; existing orphans from before this change need a one-time manual cleanup (`lsof -nP -iTCP:8080 -sTCP:LISTEN` → `kill`).
