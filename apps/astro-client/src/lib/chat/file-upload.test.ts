import { describe, expect, it } from "vitest";
import { ApiRequestError } from "@/lib/api";
import { fileUploadErrorMessage, isMissingFilesApi } from "./file-upload";

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

  it("does not special-case non-507 API errors", () => {
    const err = new ApiRequestError({ error_description: "nope" }, 400);
    expect(fileUploadErrorMessage(err, "Upload failed.")).toBe("nope");
  });
});
