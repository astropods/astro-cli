# Agent & Deployment Image Uploads

## Summary

Agents (blueprints) and deployments can now have custom avatar images uploaded through the web UI. Previously, all agents used procedurally generated SVG identities. This adds full CRUD for avatar images on both blueprints and deployments, with avatars flowing from blueprint to deployment at deploy time.

## Design

**Server — avatar lifecycle:**
- New `handlers/avatar.go` consolidates upload/delete endpoints for blueprints and deployments behind `PUT/DELETE /agents/:account/:name/avatar` and `PUT/DELETE /deployments/:id/avatar`.
- `avatar.Store` gained `CopyAgentToDeployment`, `MoveAgent`, and `DeleteAgent` methods so avatars travel with their entity during deploys, transfers, renames, and org events.
- `agentindex.Index` now tracks `avatar_version` per blueprint and exposes `AvatarVersionsByAccount` for bulk URL resolution.
- `deploymentstore.Store` tracks `avatar_version` per deployment (`schema.sql` adds the column) with increment/reset helpers.
- On first deploy, the blueprint's avatar is copied to the new deployment so read-time resolution is a simple version check — no fallback chain.

**Client — upload UX:**
- `AvatarUploadDialog` (shared component) supports both circular (account) and rectangular (agent) crop shapes via `cropShape` prop.
- Blueprint detail header shows a camera overlay for owners; uploading invalidates agent + deployment query caches.
- Deploy form and configure panel both accept an `avatar` prop for inline image preview and upload/staging.
- `AgentIdentity` renders an `<img>` when `avatarUrl` is present, falling back to the procedural SVG.
- New API client methods: `uploadBlueprintAvatar`, `uploadDeploymentAvatar`, `deleteDeploymentAvatar`.

## Migration

The `deployments` table requires a new `avatar_version INTEGER NOT NULL DEFAULT 0` column. The schema file is updated; apply the migration before deploying the new server.
