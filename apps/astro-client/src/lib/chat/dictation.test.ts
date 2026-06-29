import { describe, expect, it } from "vitest";
import type { DictationState } from "@assistant-ui/react";
import { isDictationActive } from "./dictation";

const withStatus = (status: DictationState["status"]): DictationState => ({
  status,
});

describe("isDictationActive", () => {
  it("is false when there is no dictation session", () => {
    expect(isDictationActive(undefined)).toBe(false);
  });

  it("is active while starting or running", () => {
    expect(isDictationActive(withStatus({ type: "starting" }))).toBe(true);
    expect(isDictationActive(withStatus({ type: "running" }))).toBe(true);
  });

  it("is inactive once ended, for every terminal reason (incl. error)", () => {
    // "ended" is the only terminal state; error is an ended reason, not its own
    // status — so an errored session reads as not-active and the next mic click
    // starts a fresh one rather than trying to stop a dead session.
    expect(
      isDictationActive(withStatus({ type: "ended", reason: "stopped" })),
    ).toBe(false);
    expect(
      isDictationActive(withStatus({ type: "ended", reason: "cancelled" })),
    ).toBe(false);
    expect(
      isDictationActive(withStatus({ type: "ended", reason: "error" })),
    ).toBe(false);
  });
});
