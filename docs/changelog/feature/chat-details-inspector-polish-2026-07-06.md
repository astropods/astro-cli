## Summary

The chat details panel now acts as a focused inspector for the selected agent instead of a dense deployment dashboard. The panel keeps high-signal usage, trace, and configuration context close to chat while sending deeper operational work to the agent Monitor page.

## Design

Desktop keeps the docked right-side inspector, while mobile uses the same content in a bottom sheet with viewport-bounded height and safe-area-aware footer spacing. The header keeps the agent identity persistent above local `Overview` and `Config` tabs, and surfaces only the agent-facing deployment status: `Active`, `Deploying`, `Paused`, or `Error`.

The overview tab prioritizes usage trend context and recent trace activity. Usage supports the same 7D/14D/30D range controls as Monitor and shows compact request, token, and spend metrics from the lightweight summary cache. Recent traces show status, timestamp, user, cost, and tokens, and link into the Monitor trace detail flow.

The config tab presents read-only agent configuration with tighter spacing, foreground system-prompt text, and tool rows that match the panel density.

## Migration

No user action is required. Existing chat URLs and agent Monitor URLs continue to work.
