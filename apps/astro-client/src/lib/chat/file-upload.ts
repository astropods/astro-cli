/** Shared helpers for the deployment files upload UX — used by both the chat
 *  inspector's Files panel and the chat composer's attach / drag-and-drop. */
import { ApiRequestError } from "@/lib/api";
import type { FilesApiOperation } from "@/lib/files-api-errors";

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

export function fileApiErrorMessage(
  error: unknown,
  operation: FilesApiOperation,
  fallback: string,
): string {
  if (isMissingFilesApi(error)) {
    return "File storage isn't available for this deployment yet. Its messaging sidecar may need to be updated.";
  }
  if (error instanceof ApiRequestError) {
    if (
      error.message &&
      !/^Request failed with status \d+$/.test(error.message)
    ) {
      return error.message;
    }
    switch (error.status) {
      case 400:
        return "The file request is invalid.";
      case 401:
        return "Authentication is required.";
      case 403:
        return "You don't have permission to access this file.";
      case 404:
        return operation === "list" || operation === "upload"
          ? "File storage isn't available for this deployment yet."
          : "This file is no longer available.";
      case 413:
        return "This file is too large. Choose a smaller file and try again.";
      case 507:
        return "The deployment's storage is full. Delete files to free space, then try again.";
    }
  }
  return (error instanceof Error ? error.message : "") || fallback;
}

export function fileUploadErrorMessage(
  error: unknown,
  fallback: string,
): string {
  return fileApiErrorMessage(error, "upload", fallback);
}
