import { describe, expect, it } from "vitest";
import { parseFilesApiError } from "./files-api-errors";

describe("parseFilesApiError", () => {
  it("preserves a standard Files API error", async () => {
    const response = new Response(
      JSON.stringify({
        error: "invalid_file_name",
        details: "This file name isn't supported.",
      }),
      { status: 400 },
    );

    await expect(parseFilesApiError(response, "upload")).resolves.toEqual({
      error: "invalid_file_name",
      details: "This file name isn't supported.",
    });
  });

  it("maps a legacy sidecar text response", async () => {
    const response = new Response("file too large\n", { status: 413 });

    await expect(parseFilesApiError(response, "upload")).resolves.toEqual({
      error: "file_too_large",
      details: "This file is too large. Choose a smaller file and try again.",
    });
  });

  it("does not expose Kubernetes proxy details", async () => {
    const response = new Response(
      JSON.stringify({
        kind: "Status",
        status: "Failure",
        message: "pods astro-system/internal-pod-name is forbidden",
      }),
      { status: 403 },
    );

    const error = await parseFilesApiError(response, "download");
    expect(error).toEqual({
      error: "files_unavailable",
      details: "File storage is temporarily unavailable. Try again.",
    });
    expect(error.details).not.toContain("internal-pod-name");
  });

  it("does not expose arbitrary object-store response bodies", async () => {
    const response = new Response(
      "<Error><Message>internal bucket policy detail</Message></Error>",
      { status: 500 },
    );

    const error = await parseFilesApiError(response, "upload");
    expect(error).toEqual({
      error: "file_upload_failed",
      details: "File storage is temporarily unavailable. Try again.",
    });
    expect(error.details).not.toContain("bucket policy");
  });
});
