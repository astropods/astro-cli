import { describe, it, expect } from "vitest";
import { isNonHumanIdentity, nonHumanLabel } from "./identity-kind";

describe("identity-kind", () => {
  it("treats WorkOS members and non-bot Slack users as human", () => {
    expect(isNonHumanIdentity({ kind: "member" })).toBe(false);
    expect(isNonHumanIdentity({ kind: "slack", user_details: { kind: "slack", is_bot: false } })).toBe(false);
    expect(nonHumanLabel({ kind: "member" })).toBeNull();
  });

  it("flags agents, system, and Slack bots as non-human", () => {
    expect(isNonHumanIdentity({ kind: "agent" })).toBe(true);
    expect(isNonHumanIdentity({ kind: "system" })).toBe(true);
    expect(isNonHumanIdentity({ kind: "slack", user_details: { kind: "slack", is_bot: true } })).toBe(true);
  });

  it("labels agents and bots, but not system (it has its own marker)", () => {
    expect(nonHumanLabel({ kind: "agent" })).toBe("Agent");
    expect(nonHumanLabel({ kind: "slack", user_details: { kind: "slack", is_bot: true } })).toBe("Slack bot");
    expect(nonHumanLabel({ kind: "system" })).toBeNull();
  });
});
