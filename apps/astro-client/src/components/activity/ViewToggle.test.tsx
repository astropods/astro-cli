import { describe, expect, it } from "vitest";
import { parseActivityView } from "./ViewToggle";

describe("parseActivityView", () => {
  it("returns 'agents' for null", () => {
    expect(parseActivityView(null)).toBe("agents");
  });

  it("returns 'agents' for unrecognized strings", () => {
    expect(parseActivityView("")).toBe("agents");
    expect(parseActivityView("foo")).toBe("agents");
    expect(parseActivityView("agent")).toBe("agents");
    expect(parseActivityView("USERS")).toBe("agents");
  });

  it("returns 'users' for the literal 'users'", () => {
    expect(parseActivityView("users")).toBe("users");
  });
});
