import type {
  EvaluationSetEvaluator,
  EvaluatorOutput,
  EvaluatorOutputValue,
  TraceEvaluatorResult,
} from "@/lib/api";

export interface EvaluationRow {
  key: string;
  label: string;
  description?: string;
  evaluated: boolean;
  explanation: string | null;
  output: EvaluatorOutput | null;
  confidence: number | null;
}

export function evaluationRows(
  evaluators: EvaluationSetEvaluator[],
  results: TraceEvaluatorResult[],
): EvaluationRow[] {
  if (results.length > 0) {
    return results.map((result) =>
      buildRow(
        result.key,
        result.label ?? result.key,
        result.description,
        result.output ?? null,
        result,
      ),
    );
  }

  return evaluators.map((evaluator) =>
    buildRow(
      evaluator.key,
      evaluator.label,
      evaluator.description,
      evaluator.output,
      undefined,
    ),
  );
}

export function completedOutputs(
  results: TraceEvaluatorResult[],
): EvaluatorOutputValue[] {
  return results
    .filter((result) => result.status === "completed")
    .map((result) => ({ key: result.key, value: result.value }));
}

function buildRow(
  key: string,
  label: string,
  description: string | undefined,
  output: EvaluatorOutput | null,
  result: TraceEvaluatorResult | undefined,
): EvaluationRow {
  const scored = result?.status === "completed";
  return {
    key,
    label,
    description,
    evaluated: scored,
    explanation: scored && result.explanation ? result.explanation : null,
    output,
    confidence: scored
      ? Math.round(Math.min(1, Math.max(0, result.confidence)) * 100)
      : null,
  };
}
