/** The polarity a reviewer can record for a dimension. */
export type CriterionValue = -1 | 1;

/** A judgment criterion dimension. `dimensionKey` is the stable contract with
 *  the server enum; labels and display order are frontend-owned. */
export interface JudgmentCriterionDimension {
  dimensionKey: string;
  dimensionLabel: string;
  goodLabel: string;
  badLabel: string;
  description: string;
}

// Order here is the display order. Keys must match the server enum in
// apps/astro-server/internal/judgmentstore/store.go.
export const JUDGMENT_CRITERIA: JudgmentCriterionDimension[] = [
  {
    dimensionKey: "accuracy",
    dimensionLabel: "Accuracy",
    goodLabel: "Correct info",
    badLabel: "Hallucination",
    description:
      "The answer was factually accurate, with no invented facts, citations, or capabilities.",
  },
  {
    dimensionKey: "completeness",
    dimensionLabel: "Completeness",
    goodLabel: "Complete",
    badLabel: "Incomplete",
    description:
      "The response covered everything that was asked, with no missing steps, fields, or follow-through.",
  },
  {
    dimensionKey: "instruction_following",
    dimensionLabel: "Instruction following",
    goodLabel: "Followed instruction",
    badLabel: "Ignored instruction",
    description:
      "The agent honored the explicit instructions from the user or system prompt.",
  },
  {
    dimensionKey: "scope_clarity",
    dimensionLabel: "Scope & clarity",
    goodLabel: "Clear & well-scoped",
    badLabel: "Unclear or poorly scoped",
    description:
      "The response stayed on what was asked without straying into anything the user didn't raise.",
  },
  {
    dimensionKey: "tone",
    dimensionLabel: "Tone",
    goodLabel: "Appropriate tone",
    badLabel: "Inappropriate tone",
    description:
      "The style, register, and formatting fit the user and the situation.",
  },
];

/** The selectable polarities for a dimension, in display order. */
export function criterionOptions(
  dimension: JudgmentCriterionDimension,
): { value: CriterionValue; label: string }[] {
  return [
    { value: 1, label: dimension.goodLabel },
    { value: -1, label: dimension.badLabel },
  ];
}

/** Resolves a stored criterion (dimension key + numeric value) to its display
 *  label. Returns null for unknown dimensions and for a neutral value. */
export function criterionLabelFor(dimensionKey: string, value: number): string | null {
  const dimension = JUDGMENT_CRITERIA.find((d) => d.dimensionKey === dimensionKey);
  if (!dimension || value === 0) return null;
  return value > 0 ? dimension.goodLabel : dimension.badLabel;
}



