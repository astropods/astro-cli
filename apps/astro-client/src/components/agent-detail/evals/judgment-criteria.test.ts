import { describe, it, expect } from "vitest";
import {
  criterionLabelFor,
  predictedCriteria,
  predictionCriterionAssessment,
  predictionCriterionValue,
} from "./judgment-criteria";

describe("criterionLabelFor", () => {
  it("returns the good label for a positive value", () => {
    expect(criterionLabelFor("accuracy", 1)).toBe("Correct info");
    expect(criterionLabelFor("completeness", 1)).toBe("Complete");
  });

  it("returns null for a neutral value", () => {
    expect(criterionLabelFor("tone", 0)).toBeNull();
  });

  it("returns the bad label for a negative value", () => {
    expect(criterionLabelFor("accuracy", -1)).toBe("Hallucination");
    expect(criterionLabelFor("instruction_following", -1)).toBe("Ignored instruction");
  });

  it("returns null for an unknown dimension key", () => {
    expect(criterionLabelFor("nonexistent", 1)).toBeNull();
  });
});

describe("prediction criteria", () => {
  const criteria = [
    { dimension_key: "accuracy", dimension_value: -0.8 },
    { dimension_key: "completeness", dimension_value: -0.4 },
    { dimension_key: "instruction_following", dimension_value: -0.75 },
    { dimension_key: "scope_clarity", dimension_value: 0.25 },
    { dimension_key: "tone", dimension_value: 0.3 },
  ];

  it("gets criterion values and defaults missing dimensions to neutral", () => {
    expect(predictionCriterionValue(criteria, "accuracy")).toBe(-0.8);
    expect(predictionCriterionValue(criteria, "unknown")).toBe(0);
  });

  it.each([
    [-0.8, "rejected"],
    [-0.75, "warning"],
    [0.25, "warning"],
    [0.3, "accepted"],
  ] as const)("classifies %s as %s", (value, assessment) => {
    expect(predictionCriterionAssessment(value)).toBe(assessment);
  });

  it("pre-fills accepted and rejected criteria with their polarity", () => {
    expect(predictedCriteria(criteria)).toEqual([
      { dimension_key: "accuracy", value: -1 },
      { dimension_key: "tone", value: 1 },
    ]);
  });
});
