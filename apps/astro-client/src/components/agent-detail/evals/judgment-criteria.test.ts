import { describe, it, expect } from "vitest";
import { criterionLabelFor } from "./judgment-criteria";

describe("criterionLabelFor", () => {
  it("returns the good label for a non-negative value", () => {
    expect(criterionLabelFor("accuracy", 1)).toBe("Correct info");
    expect(criterionLabelFor("completeness", 1)).toBe("Complete");
    expect(criterionLabelFor("tone", 0)).toBe("Appropriate tone");
  });

  it("returns the bad label for a negative value", () => {
    expect(criterionLabelFor("accuracy", -1)).toBe("Hallucination");
    expect(criterionLabelFor("instruction_following", -1)).toBe("Ignored instruction");
  });

  it("returns null for an unknown dimension key", () => {
    expect(criterionLabelFor("nonexistent", 1)).toBeNull();
  });
});
