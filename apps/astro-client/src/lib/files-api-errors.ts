export type FilesApiOperation =
  | "list"
  | "usage"
  | "upload"
  | "download"
  | "delete";

export interface FilesApiErrorPayload {
  error: string;
  details?: string;
  error_description?: string;
}

const knownCodes = new Set([
  "authentication_required",
  "file_access_forbidden",
  "file_create_failed",
  "file_delete_failed",
  "file_download_failed",
  "file_list_failed",
  "file_not_found",
  "file_read_failed",
  "file_storage_unavailable",
  "file_too_large",
  "file_upload_failed",
  "files_unavailable",
  "insufficient_storage",
  "invalid_file_name",
  "invalid_file_request",
]);

const legacyMessages: Record<string, FilesApiErrorPayload> = {
  "invalid file name": {
    error: "invalid_file_name",
    details: "This file name isn't supported.",
  },
  "invalid file key": {
    error: "invalid_file_request",
    details: "The file key is invalid.",
  },
  "invalid file size": {
    error: "invalid_file_request",
    details: "The file request is invalid.",
  },
  "file too large": {
    error: "file_too_large",
    details: "This file is too large. Choose a smaller file and try again.",
  },
  "not enough storage available on the deployment volume": {
    error: "insufficient_storage",
    details:
      "The deployment's storage is full. Delete files to free space, then try again.",
  },
  "file storage is not enabled": {
    error: "file_storage_unavailable",
    details: "File storage isn't available for this deployment yet.",
  },
  "file not found": {
    error: "file_not_found",
    details: "This file is no longer available.",
  },
  "file content not found": {
    error: "file_not_found",
    details: "This file is no longer available.",
  },
};

function fallbackFilesError(
  status: number,
  operation: FilesApiOperation,
): FilesApiErrorPayload {
  switch (status) {
    case 400:
      return {
        error: "invalid_file_request",
        details: "The file request is invalid.",
      };
    case 401:
      return {
        error: "authentication_required",
        details: "Authentication is required.",
      };
    case 403:
      return {
        error: "file_access_forbidden",
        details: "You don't have permission to access this file.",
      };
    case 404:
      return operation === "list" || operation === "upload"
        ? {
            error: "file_storage_unavailable",
            details: "File storage isn't available for this deployment yet.",
          }
        : {
            error: "file_not_found",
            details: "This file is no longer available.",
          };
    case 413:
      return {
        error: "file_too_large",
        details: "This file is too large. Choose a smaller file and try again.",
      };
    case 507:
      return {
        error: "insufficient_storage",
        details:
          "The deployment's storage is full. Delete files to free space, then try again.",
      };
    default: {
      const errorByOperation: Record<FilesApiOperation, string> = {
        list: "file_list_failed",
        usage: "files_unavailable",
        upload: "file_upload_failed",
        download: "file_download_failed",
        delete: "file_delete_failed",
      };
      return {
        error: errorByOperation[operation],
        details: "File storage is temporarily unavailable. Try again.",
      };
    }
  }
}

function isKubernetesFailure(value: unknown): boolean {
  if (!value || typeof value !== "object") return false;
  const body = value as Record<string, unknown>;
  return body.kind === "Status" && body.status === "Failure";
}

function parseKnownPayload(value: unknown): FilesApiErrorPayload | null {
  if (!value || typeof value !== "object") return null;
  const body = value as Record<string, unknown>;

  const code = typeof body.error === "string" ? body.error.trim() : "";
  if (knownCodes.has(code)) {
    const detailValue =
      typeof body.details === "string"
        ? body.details
        : typeof body.error_description === "string"
          ? body.error_description
          : "";
    const details = detailValue.trim();
    return {
      error: code,
      ...(details && details.length <= 300 ? { details } : {}),
    };
  }

  return legacyMessages[code] ?? null;
}

/**
 * Parses failures from astro-server, a direct local messaging sidecar, or a
 * presigned object store without exposing arbitrary upstream response text.
 */
export async function parseFilesApiError(
  response: Response,
  operation: FilesApiOperation,
): Promise<FilesApiErrorPayload> {
  const text = await response.text().catch(() => "");
  const trimmed = text.trim();

  if (trimmed) {
    try {
      const value: unknown = JSON.parse(trimmed);
      // Kubernetes Status objects can contain cluster identities and internal
      // proxy paths. Never surface their message or treat them as user authz.
      if (isKubernetesFailure(value)) {
        return {
          error: "files_unavailable",
          details: "File storage is temporarily unavailable. Try again.",
        };
      }
      const parsed = parseKnownPayload(value);
      if (parsed) return parsed;
    } catch {
      const firstLine = trimmed.split("\n", 1)[0];
      const legacy = legacyMessages[firstLine];
      if (legacy) return legacy;
    }
  }

  return fallbackFilesError(response.status, operation);
}
