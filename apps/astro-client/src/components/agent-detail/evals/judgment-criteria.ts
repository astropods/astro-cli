import type { DatasetJudgmentVerdict, JudgmentCriterion } from "@/lib/api";

/** good/bad verdicts get the criteria panel and stay on screen until Done;
 *  neutral has no criteria and commits immediately. */
export function verdictHasCriteria(
  verdict: DatasetJudgmentVerdict,
): verdict is "good" | "bad" {
  return verdict === "good" || verdict === "bad";
}

/** A judgment criterion dimension. `dimensionKey` is the stable contract with
 *  the server enum; labels and display order are frontend-owned. */
export interface JudgmentCriterionDimension {
  dimensionKey: string;
  goodLabel: string;
  badLabel: string;
}

// Order here is the display order. Keys must match the server enum in
// apps/astro-server/internal/judgmentstore/store.go.
export const JUDGMENT_CRITERIA: JudgmentCriterionDimension[] = [
  { dimensionKey: "accuracy", goodLabel: "Correct info", badLabel: "Hallucination" },
  { dimensionKey: "completeness", goodLabel: "Complete", badLabel: "Incomplete" },
  {
    dimensionKey: "instruction_following",
    goodLabel: "Followed instruction",
    badLabel: "Ignored instruction",
  },
  {
    dimensionKey: "scope_clarity",
    goodLabel: "Clear & well-scoped",
    badLabel: "Unclear or poorly scoped",
  },
  { dimensionKey: "tone", goodLabel: "Appropriate tone", badLabel: "Inappropriate tone" },
];

/** Human review sends an absolute signal: 1 for good, -1 for bad. */
export function criterionValueForVerdict(verdict: "good" | "bad"): number {
  return verdict === "good" ? 1 : -1;
}

export function criterionLabel(
  dimension: JudgmentCriterionDimension,
  verdict: "good" | "bad",
): string {
  return verdict === "good" ? dimension.goodLabel : dimension.badLabel;
}

/** Maps the selected dimension keys to the request criteria for a verdict. */
export function toCriteria(
  selectedKeys: Iterable<string>,
  verdict: "good" | "bad",
): JudgmentCriterion[] {
  const value = criterionValueForVerdict(verdict);
  return Array.from(selectedKeys, (dimension_key) => ({ dimension_key, value }));
}
