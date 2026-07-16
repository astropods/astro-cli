# Agent file upload/download

## Summary

Users can attach files to a deployment chat message and the agent receives them
as the immediate context for that turn; files the agent produces come back as
download chips on its reply — the Claude / Claude Code model. This replaces an
earlier standalone "Files" drawer that stored files decoupled from the
conversation (which produced duplicate listings and no message association). The
drawer remains as a secondary, read-only-ish view of everything on the
deployment's files volume.

## Design

**Storage.** Files live on a dedicated `files` subPath slice of the agent's
persistent volume, parallel to the `messaging` slice that holds chat history. The
messaging sidecar mounts it at `FILES_DIR`; the agent container mounts the whole
volume and sees the same bytes at `AGENT_FILES_DIR` (`/data/files`), now injected
as first-class deploy config by astro-server (K8s) and the CLI (`ast dev`).

**Transport.** The client uploads bytes through the existing files API
(reserve-key + PUT) on send; only the opaque key + display metadata ride the chat
message (no inlined bytes, so the send stays small and the proxy's body cap
holds). astro-server proxies the send verbatim.

**Message contract.** The messaging protocol already modeled per-message
attachments; this wires them for the platform ("web") chat end to end:
- Send: `POST …/messages` accepts `attachments: [{key}]`; the sidecar re-reads
  authoritative metadata from the file store — rejecting unknown keys, keys owned
  by another user, and sends over a fixed per-message attachment cap — and sets
  `Message.attachments` on the turn forwarded to the agent. A new
  `Attachment.storage_key` carries the opaque key; the agent resolves the local
  path from `AGENT_FILES_DIR` (the `url` field stays a download URL for the client
  and a future presigned-object store).
- Persist/read: a `messages.attachments` JSON column (added with a guarded
  `ALTER TABLE` since there's no migration framework) round-trips attachments
  through history and the SSE stream, so chips survive reload.
- Agent replies: the sidecar now consumes `ResponseAttachment` on agent output
  (previously web-ignored) and emits/persists it, rendering as reply download
  chips.

**Access control.** Files are scoped to the user who uploaded them, not to the
deployment. The file store records an `UploadedBy` owner; every files-API read
(list/get/content/delete) and every chat attachment reference is checked against
the requesting user, returning "not found" for another user's file so existence
never leaks across users of the same account. Agent-produced files are attributed
to the conversation's owner so the reply's download chip works for them and no one
else. Ownership is **immutable** once a real user owns a file: attribution is a
compare-and-set that only claims unowned (agent-written) files, so a shared agent
that echoes another user's storage key while replying in a different conversation
cannot transfer that file across users, and two concurrent responses cannot race
to reassign it.

**Filesystem containment.** Because an agent can write arbitrary entries into the
shared files directory, the filesystem store never follows symlinks: metadata is
read with `Lstat`, blobs are opened `O_NOFOLLOW` and validated by fstat on the
open handle (no check/open race), and writes create the temp file
`O_EXCL|O_NOFOLLOW` and commit by rename. An agent that drops
`output.txt -> /etc/passwd` therefore has it neither adopted nor served — the
files API can only ever return bytes from inside the store.

**Agent SDK.** The TS and Python bridges now pass inbound attachments into
`StreamOptions.attachments` and add an `onFile` hook that delivers agent-produced
files on the END chunk. The on-disk `path` is populated only when the message
carried the opaque storage key (which locates the `<key>.blob` on the shared
volume); without it the agent scans its files dir rather than trusting a path
derived from the filename. Existing filesystem agents keep working unchanged.

**Client.** The chat composer uses assistant-ui's attachment adapter: paperclip,
drag-and-drop, and paste stage removable chips; bytes upload on send. Sent
messages render file chips; assistant replies render download chips.

**Dedup.** The files store no longer lists an agent-written plain file twice when
its name matches an API upload — the managed record wins.

**Storage capacity.** The sidecar exposes `GET …/files/usage` (a `statfs` of the
shared volume — chat DB + files + agent outputs together, the real capacity
uploads compete for); astro-server proxies it at `/deployments/:id/files/usage`.
An upload that wouldn't fit (declared size + reserve) is rejected up front with
`507`, surfaced in the composer and Files panel as "the deployment's storage is
full" rather than a generic error. Uploads follow a **reserved → ready**
lifecycle so the reserve is enforceable: create reserves the key and capacity by
the declared size, the server-received PUT treats that declared size as a hard
ceiling (a client can't declare one byte to pass the check then stream 100 MiB)
and rechecks capacity before writing, and the file is promoted to ready only once
its bytes are committed. Only ready files are listable, downloadable, or
attachable, so an abandoned or in-flight upload never appears as a broken download
chip. A `StorageCapacityBanner` on the chat screen
and the agent-detail Monitor tab warns above 85% usage (error-toned above 95%)
and auto-clears once space is freed. All of it works in both cluster and `ast dev`
without any metrics backend.

**Local dev (`ast dev`).** This branch merges the CLI-embedded chat SPA change
(the deprecated in-sidecar playground is retired in favour of astro-client's real
chat page served by the CLI). The CLI's `chatui` proxy is extended to forward the
`/files/*` contract (including `/files/usage`) to the local sidecar, so file
upload/download and the capacity banner behave in `ast dev` exactly as in a
cluster deployment.

## Migration

None required. The `messages.attachments` column is added automatically on
existing deployment volumes. Deployments keep working with no spec change; the
feature activates wherever the messaging web adapter and files volume are present.
