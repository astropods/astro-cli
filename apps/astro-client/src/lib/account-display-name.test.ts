import { describe, expect, it } from "vitest";
import { ORG_DISPLAY_NAME_MAX_LENGTH } from "@/lib/constants";
import {
  getDisplayNameEmptyError,
  getDisplayNameValidationError,
  isDisplayNameRequired,
} from "./account-display-name";

describe("account display-name validation", () => {
  it("counts astral-plane characters the same way the server does", () => {
    expect(
      getDisplayNameValidationError(
        "🪐".repeat(ORG_DISPLAY_NAME_MAX_LENGTH),
        "organization",
      ),
    ).toBeNull();

    expect(
      getDisplayNameValidationError(
        "🪐".repeat(ORG_DISPLAY_NAME_MAX_LENGTH + 1),
        "organization",
      ),
    ).toBe(
      `Organization names cannot exceed ${ORG_DISPLAY_NAME_MAX_LENGTH} characters.`,
    );
  });

  it("returns kind-specific empty display-name messages", () => {
    expect(getDisplayNameEmptyError("personal")).toBe(
      "Display name can't be empty",
    );
    expect(getDisplayNameEmptyError("organization")).toBe(
      "Organization name can't be empty",
    );
  });

  it("requires display names for all account kinds", () => {
    expect(isDisplayNameRequired("personal")).toBe(true);
    expect(isDisplayNameRequired("organization")).toBe(true);
  });
});
