# ast project start --local: kill the full agent process tree on Ctrl+C

## Summary

`ast project start --local` (alias: `ast dev --local`) spawns the agent as a host process via `sh -c "bun --watch run start"` (or `python3 -m agent.main`) inside its own process group. On shutdown the CLI only killed the immediate `sh` process, leaving `bun --watch` workers and any agent-spawned grandchildren running as orphans — they kept holding ports, file watches, and credentials until killed by hand.

## Design

The agent is launched with `SysProcAttr{Setpgid: true}` so the whole tree shares a process group rooted at the `sh` pid. The fix signals that group instead of a single pid:

- `agentCmd.Cancel` is overridden so context cancellation sends `SIGTERM` to `-pgid` rather than calling `Process.Kill()` on `sh` alone.
- On Ctrl+C, the shutdown path sends `SIGTERM` to the group, waits up to 3s for `Wait()` to return, and falls back to `SIGKILL` on the group if the children ignore TERM.
- `agentCancel()` is moved to *after* the group has been reaped, so the previous tear-down ordering (which could let the sh pid be reused before we signaled it) is gone.

## Migration

None. Existing `ast project start --local` users get clean shutdown automatically.
