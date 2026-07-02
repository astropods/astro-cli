## Summary

Addresses [#1446](https://github.com/astropods/astro/issues/1446) and [#1527](https://github.com/astropods/astro/issues/1527).

Existing deployments now reopen the configure panel with their saved custom web access, knowledge store mode, and data persistence settings intact. This fixes the panel defaulting the custom interface back to protected, losing the Local/Shared knowledge selection, and losing previously resolved provisioning values.

## Design

The deploy form now derives first-render defaults from the prefetched template response when one is available. For configure flows, explicit empty adapter selections are preserved instead of being treated as missing data, and resolved agent provisioning is seeded into the advanced resources fields.

The interface validation now matches how the configure panel is meant to work:

- Before this change, the form could forget the saved interface choices and fall back to defaults, so a custom web agent could come back as protected or with the wrong messaging selection.
- Messaging-only agents still need at least one messaging adapter, such as Astro Chat or Slack, on both fresh deploy and reconfigure.
- Agents that ship their own custom web interface may omit messaging adapters, because that custom interface can be the way users reach the agent.
- Protected custom interfaces remain valid even with no saved grants. Protection routes the custom interface through Astro sign-in today; saved grants are recorded for the interface but are not enforced by the platform yet.
- Knowledge store rows now keep Local vs Shared as explicit form state instead of re-guessing it from the selected store ARN on every render. Local deployments reopen as Local, shared bindings reopen as Shared, and switching modes counts as one pending redeploy change for that row.
- Shared knowledge mode now requires an actual shared store selection before save or redeploy. Before this guard, a user could choose Shared, leave the store dropdown empty, and attempt a redeploy with no usable knowledge binding. The submit button stays enabled, but submit shows an inline error telling the user to select a shared store or switch back to Local.

Dirty-state detection now includes provisioning edits, so CPU, memory, mount path, and storage changes participate in the redeploy state the same way variables, schedules, adapters, and auth settings do.

## Migration

No migration is required. Existing deployments already store these values; the client now reflects them correctly when the configure panel loads.
