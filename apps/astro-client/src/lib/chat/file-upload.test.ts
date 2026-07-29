import { describe, expect, it } from "vitest";
import { ApiRequestError } from "@/lib/api";
import {
  fileApiErrorMessage,
  fileUploadErrorMessage,
  isMissingFilesApi,
} from "./file-upload";

describe("fileUploadErrorMessage", () => {
  it("maps 507 Insufficient Storage to an actionable storage-full message", () => {
    const err = new ApiRequestError(
      { error_description: "Request failed with status 507" },
      507,
    );
    expect(fileUploadErrorMessage(err, "Upload failed.")).toBe(
      "The deployment's storage is full. Delete files to free space, then try again.",
    );
  });

  it("explains a missing files API (non-JSON sidecar response)", () => {
    const err = new SyntaxError("Unexpected token '<'");
    expect(isMissingFilesApi(err)).toBe(true);
    expect(fileUploadErrorMessage(err, "Upload failed.")).toMatch(
      /File storage isn't available/,
    );
  });

  it("falls back for other errors, preferring the error message", () => {
    expect(fileUploadErrorMessage(new Error("boom"), "Upload failed.")).toBe(
      "boom",
    );
    expect(fileUploadErrorMessage(null, "Upload failed.")).toBe("Upload failed.");
  });

  it("preserves a specific Files API message", () => {
    const err = new ApiRequestError({ error_description: "nope" }, 400);
    expect(fileUploadErrorMessage(err, "Upload failed.")).toBe("nope");
  });

  it("uses operation-specific fallbacks for a generic 404", () => {
    const err = new ApiRequestError({}, 404);
    expect(fileApiErrorMessage(err, "upload", "Upload failed.")).toMatch(
      /storage isn't available/,
    );
    expect(fileApiErrorMessage(err, "download", "Download failed.")).toBe(
      "This file is no longer available.",
    );
  });
});
