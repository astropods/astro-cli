import type {
  DatasetJudgmentVerdict,
  JudgmentCriterion,
  ReviewQueuePredictionCriterion,
} from "@/lib/api";

type BinaryJudgmentVerdict = "good" | "bad";

/** good/bad verdicts get the criteria panel and stay on screen until Done;
 *  neutral has no criteria and commits immediately. */
export function verdictHasCriteria(
  verdict: DatasetJudgmentVerdict,
): verdict is BinaryJudgmentVerdict {
  return verdict === "good" || verdict === "bad";
}

/** A judgment criterion dimension. `dimensionKey` is the stable contract with
 *  the server enum; labels and display order are frontend-owned. */
export interface JudgmentCriterionDimension {
  dimensionKey: string;
  dimensionLabel: string;
  goodLabel: string;
  badLabel: string;
  goodTooltip: string;
  badTooltip: string;
}

// Order here is the display order. Keys must match the server enum in
// apps/astro-server/internal/judgmentstore/store.go.
export const JUDGMENT_CRITERIA: JudgmentCriterionDimension[] = [
  {
    dimensionKey: "accuracy",
    dimensionLabel: "Accuracy",
    goodLabel: "Correct info",
    badLabel: "Hallucination",
    goodTooltip:
      "The answer was factually accurate, with no invented facts, citations, or capabilities.",
    badTooltip:
      "The answer contained factual errors or invented facts, citations, or capabilities that don't exist.",
  },
  {
    dimensionKey: "completeness",
    dimensionLabel: "Completeness",
    goodLabel: "Complete",
    badLabel: "Incomplete",
    goodTooltip:
      "The response covered everything that was asked, with no missing steps, fields, or follow-through.",
    badTooltip:
      "The response left out part of what was asked, such as missing steps, fields, or follow-through.",
  },
  {
    dimensionKey: "instruction_following",
    dimensionLabel: "Instruction following",
    goodLabel: "Followed instruction",
    badLabel: "Ignored instruction",
    goodTooltip:
      "The agent honored the explicit instructions from the user or system prompt.",
    badTooltip:
      "The agent overlooked or contradicted an explicit instruction from the user or system prompt.",
  },
  {
    dimensionKey: "scope_clarity",
    dimensionLabel: "Scope & clarity",
    goodLabel: "Clear & well-scoped",
    badLabel: "Unclear or poorly scoped",
    goodTooltip:
      "The response stayed on what was asked without straying into anything the user didn't raise.",
    badTooltip:
      "The response strayed from what was asked or addressed something the user didn't raise.",
  },
  {
    dimensionKey: "tone",
    dimensionLabel: "Tone",
    goodLabel: "Appropriate tone",
    badLabel: "Inappropriate tone",
    goodTooltip:
      "The style, register, and formatting fit the user and the situation.",
    badTooltip:
      "The style, register, or formatting didn't fit the user or the situation.",
  },
];

/** Human review sends an absolute signal: 1 for good, -1 for bad. */
export function criterionValueForVerdict(verdict: BinaryJudgmentVerdict): number {
  return verdict === "good" ? 1 : -1;
}

export function criterionLabel(
  dimension: JudgmentCriterionDimension,
  verdict: BinaryJudgmentVerdict,
): string {
  return verdict === "good" ? dimension.goodLabel : dimension.badLabel;
}

export function criterionTooltip(
  dimension: JudgmentCriterionDimension,
  verdict: BinaryJudgmentVerdict,
): string {
  return verdict === "good" ? dimension.goodTooltip : dimension.badTooltip;
}

/** Resolves a stored criterion (dimension key + numeric value) to its display
 *  label. value: 1 = good, -1 = bad. Returns null for unknown dimensions. */
export function criterionLabelFor(dimensionKey: string, value: number): string | null {
  const dimension = JUDGMENT_CRITERIA.find((d) => d.dimensionKey === dimensionKey);
  if (!dimension) return null;
  return criterionLabel(dimension, value >= 0 ? "good" : "bad");
}

/** Maps the selected dimension keys to the request criteria for a verdict. */
export function toCriteria(
  selectedKeys: Iterable<string>,
  verdict: BinaryJudgmentVerdict,
): JudgmentCriterion[] {
  const value = criterionValueForVerdict(verdict);
  return Array.from(selectedKeys, (dimension_key) => ({ dimension_key, value }));
}

export type PredictionCriterionAssessment =
  | "accepted"
  | "warning"
  | "rejected";

/** Returns the predicted score for a criterion, defaulting missing dimensions
 *  to the neutral middle band. */
export function predictionCriterionValue(
  criteria: ReviewQueuePredictionCriterion[],
  dimensionKey: string,
): number {
  return (
    criteria.find(({ dimension_key }) => dimension_key === dimensionKey)
      ?.dimension_value ?? 0
  );
}

/** Converts a model score into the shared presentation and selection bands. */
export function predictionCriterionAssessment(
  value: number,
): PredictionCriterionAssessment {
  if (value > 0.25) return "accepted";
  if (value < -0.75) return "rejected";
  return "warning";
}

/** Selects predicted criterion dimensions that support the agreed verdict. */
export function predictedCriterionKeysForVerdict(
  criteria: ReviewQueuePredictionCriterion[],
  verdict: BinaryJudgmentVerdict,
): string[] {
  return JUDGMENT_CRITERIA.filter(({ dimensionKey }) => {
    const assessment = predictionCriterionAssessment(
      predictionCriterionValue(criteria, dimensionKey),
    );
    return verdict === "good"
      ? assessment === "accepted"
      : assessment === "rejected";
  }).map(({ dimensionKey }) => dimensionKey);
}
