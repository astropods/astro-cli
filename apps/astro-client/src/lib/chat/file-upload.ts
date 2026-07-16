/** Shared helpers for the deployment files upload UX — used by both the chat
 *  inspector's Files panel and the chat composer's attach / drag-and-drop. */
import { ApiRequestError } from "@/lib/api";

// A non-JSON response (typically the messaging sidecar's playground index.html)
// means the deployment's sidecar predates the files API. Surface that instead of
// the raw "Unexpected token '<'" JSON parse error.
export function isMissingFilesApi(error: unknown): boolean {
  const message = error instanceof Error ? error.message : String(error ?? "");
  return (
    error instanceof SyntaxError ||
    message.includes("is not valid JSON") ||
    message.includes("<!doctype") ||
    message.includes("Unexpected token")
  );
}

export function fileUploadErrorMessage(
  error: unknown,
  fallback: string,
): string {
  if (isMissingFilesApi(error)) {
    return "File storage isn't available for this deployment yet. Its messaging sidecar may need to be updated.";
  }
  // 507 Insufficient Storage — the deployment volume is (near) full. Give an
  // actionable message instead of the raw "request failed with status 507".
  if (error instanceof ApiRequestError && error.status === 507) {
    return "The deployment's storage is full. Delete files to free space, then try again.";
  }
  return (error instanceof Error ? error.message : "") || fallback;
}
