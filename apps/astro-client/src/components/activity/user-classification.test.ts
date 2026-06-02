import { describe, expect, it } from "vitest";
import { isSlackUserId } from "./user-classification";

describe("isSlackUserId", () => {
  it("matches Slack user ids — bare form is the only format on disk", () => {
    expect(isSlackUserId("U07ABCDEF")).toBe(true);     // 9 chars (lower bound)
    expect(isSlackUserId("U01234567890")).toBe(true); // 12 chars (upper bound)
  });

  it("rejects WorkOS user ids", () => {
    expect(isSlackUserId("user_01HXX")).toBe(false);
  });

  it("rejects empty / lowercase / wrong-prefix strings", () => {
    expect(isSlackUserId("")).toBe(false);
    expect(isSlackUserId("u01abcdef")).toBe(false);
  });

  // The {8,11} bound on the user-id portion is deliberate: looser would
  // false-positive on any arbitrary `U…` string that happens to start
  // with U + alphanumerics; tighter would drop real Slack ids. Pin both
  // boundaries so future regex tweaks have to stay inside the window.
  it("rejects user ids shorter than the {8,11} bound", () => {
    expect(isSlackUserId("U07ABC")).toBe(false);       // 6 chars — too short
    expect(isSlackUserId("U07ABCD")).toBe(false);      // 7 chars — still too short
  });

  it("rejects user ids longer than the {8,11} bound", () => {
    expect(isSlackUserId("U0123456789012")).toBe(false); // 14 chars — too long
  });

  // The previously-supported `slack:<team>:<user>` form is gone — messaging
  // now writes only the bare slack id. A stray namespaced string in the
  // pipeline should fall through to "Unidentified" rather than silently
  // double-classify as a Slack row alongside a bare row for the same human.
  it("rejects the legacy namespaced form (no longer emitted)", () => {
    expect(isSlackUserId("slack:T07XYZ:U07ABCDEF")).toBe(false);
  });
});
