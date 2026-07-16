/** assistant-ui AttachmentAdapter for deployment chat: files attached in the
 *  composer upload to the deployment's files store on send, and the resulting
 *  key + metadata is smuggled into the completed attachment's content so the
 *  runtime's onNew can forward it as a message attachment (the agent's immediate
 *  context for that turn). The byte transport is the existing files API. */
import type {
  AttachmentAdapter,
  CompleteAttachment,
  PendingAttachment,
} from "@assistant-ui/react";
import type { ApiClient, ChatAttachment } from "@/lib/api";
import { fileUploadErrorMessage } from "@/lib/chat/file-upload";

/** Content-part name carrying the uploaded file reference from the adapter's
 *  send() to the runtime's onNew. Kept in sync with readAttachmentRef below. */
export const ASTRO_FILE_PART = "astro-file";

/** Reads the ChatAttachment a completed attachment carries, or null if absent
 *  (e.g. an attachment from a different adapter). */
export function readAttachmentRef(attachment: {
  content?: readonly { type: string; name?: string; data?: unknown }[];
}): ChatAttachment | null {
  const part = attachment.content?.find(
    (p) => p.type === "data" && p.name === ASTRO_FILE_PART,
  );
  const data = part?.data as Partial<ChatAttachment> | undefined;
  if (!data || typeof data.key !== "string") return null;
  return {
    key: data.key,
    name: data.name ?? data.key,
    content_type: data.content_type ?? "application/octet-stream",
    size: typeof data.size === "number" ? data.size : 0,
  };
}

function attachmentKind(file: File): "image" | "file" {
  return file.type.startsWith("image/") ? "image" : "file";
}

export function createDeploymentAttachmentAdapter(
  api: ApiClient,
  deploymentId: string,
): AttachmentAdapter {
  return {
    // "*" is assistant-ui's accept-all token (fileMatchesAccept); "*/*" is NOT
    // treated as a wildcard. The store bounds size and the sidecar validates.
    accept: "*",

    // Register the chip without uploading — the bytes go up on send() so a file
    // added and then removed before sending never touches the store.
    async add({ file }): Promise<PendingAttachment> {
      return {
        id: crypto.randomUUID(),
        type: attachmentKind(file),
        name: file.name,
        contentType: file.type || "application/octet-stream",
        file,
        status: { type: "requires-action", reason: "composer-send" },
      };
    },

    // Nothing is persisted until send(), so removing a pending chip is local.
    async remove(): Promise<void> {},

    // Upload the bytes and hand back the file reference in a data content part
    // that onNew reads to build the message's attachment list.
    async send(attachment: PendingAttachment): Promise<CompleteAttachment> {
      let meta;
      try {
        meta = await api.uploadDeploymentFile(deploymentId, attachment.file);
      } catch (err) {
        throw new Error(fileUploadErrorMessage(err, "Upload failed."));
      }
      const ref: ChatAttachment = {
        key: meta.key,
        name: meta.name,
        content_type: meta.content_type,
        size: meta.size,
      };
      return {
        id: attachment.id,
        type: attachment.type,
        name: meta.name,
        contentType: meta.content_type,
        status: { type: "complete" },
        content: [{ type: "data", name: ASTRO_FILE_PART, data: ref }],
      };
    },
  };
}
