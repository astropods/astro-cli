# Summary

Local `ast dev` gave every agent one global `agent-data` volume, so the messaging
sidecar's chat history and uploaded files leaked between agents: one agent's
conversations and files would show up in another agent's chat.

# Design

The compose builder declared the `/data` volume with a fixed `Name: "agent-data"`,
which tells Docker Compose not to project-scope it, so every `ast dev` agent
mounted the same global volume. The sidecar keeps its chat SQLite DB
(`/data/chat.db`) and the files API's data (`/data/files`) there, so both were
shared across agents.

Giving the volume a project-scoped name (`<project>-agent-data`) isolates it per
agent. The agent container and its messaging sidecar still share it within one
project (the local-dev analogue of the shared PVC in production), but different
agents now get separate data. Leaving the name empty is not an option: the
programmatic compose builder does not auto-derive a name, so Compose rejects the
empty value at `up`.

# Migration

Local dev only. On the first `ast dev` after this change each agent gets a fresh
per-project volume, so existing chat history and files on the old global
`agent-data` volume won't carry over. Reclaim that space with
`docker volume rm agent-data` once no `ast dev` session is using it.
