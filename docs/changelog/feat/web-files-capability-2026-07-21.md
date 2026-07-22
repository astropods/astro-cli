# Summary

Gates the chat composer's file-upload affordance and the inspector's Files tab on
whether the agent actually supports files, so agents that never wire up the files
API no longer show upload controls that silently do nothing.

# Design

The messaging sidecar reports a `capabilities.files` flag on `GET /api/agent/config`
(proxied through astro-server), true only when the sidecar has file storage wired
AND the agent declared it consumes attachments. The client reads that one signal:

- **Composer.** `DeploymentChatRuntimeProvider` registers the attachment adapter and
  shows the paperclip + drag-and-drop only when `capabilities.files` is true; the
  gate is published through the streaming context so the runtime and the UI stay in
  lockstep.
- **Files tab.** `ChatInspectorPanel` hides the Files tab entirely when the flag is
  false, redirecting to Overview if it was the active tab.
- **Default hidden.** When the field is absent (older sidecar) the client treats it
  as false, so nothing regresses and the controls simply stay hidden until the
  sidecar reports the capability.

# Migration

None for the client. The upload controls appear only once the deployment runs a
messaging sidecar that reports `capabilities.files` and an agent that declares the
capability (see the paired `astropods/messaging` change). Until both ship, the
field is absent and the controls stay hidden.
