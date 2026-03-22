# Archive Blueprint

## Summary

Owners could not remove agent blueprints they no longer maintain from listings. Deleting would break existing deployments referencing those blueprints. This adds a soft-delete (archive) flow: archived blueprints are hidden from the catalog and profile but preserved in the database so existing deployments continue to work.

## Design

A three-dot menu (`EllipsisVertical`) on `AgentCard` shows an "Archive" option (with `Archive` icon) only when an `onArchive` prop is provided. `AccountProfile` passes `onArchive` for members viewing their own blueprints; non-members and deployed-agent cards never see the menu.

Clicking "Archive" opens an `ArchiveAgentDialog` (`ConfirmationDialog` wrapper) requiring the user to type the agent name. On confirmation the `useArchiveAgent` mutation calls `POST /api/v1/agents/:account/:name/archive`, then invalidates both per-account and global agent caches.

Backend: `ArchiveAgent` handler on `astro-server` calls `agentindex.Archive(accountID, name)` which sets `archived_at = NOW()` on the agent row (soft-delete). All list queries (`List`, `ListForAccount`, `ListPublicAgents`) filter `WHERE archived_at IS NULL`. The `Get` method still returns archived agents (needed by existing deployments). Route is under `agentWriteRoutes` (bearer auth).

## Migration

Required: `ALTER TABLE agents ADD COLUMN archived_at TIMESTAMP;`

The column defaults to NULL (not archived). No backfill needed.
