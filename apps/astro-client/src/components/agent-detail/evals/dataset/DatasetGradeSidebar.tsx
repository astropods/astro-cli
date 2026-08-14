import { InfoHint } from "@/components/InfoHint";
import { TooltipProvider } from "@/components/ui/tooltip";
import { ProgressBar } from "@/components/ui/progress-bar";
import type { EvalDatasetResponse } from "@/lib/api";
import { JUDGMENT_CRITERIA } from "../judgment-criteria";

function greatestCommonDivisor(left: number, right: number): number {
  let a = Math.abs(left);
  let b = Math.abs(right);
  while (b !== 0) {
    [a, b] = [b, a % b];
  }
  return a;
}

function formatRatio(positive: number, negative: number): string {
  const divisor = greatestCommonDivisor(positive, negative);
  return `${positive / divisor}:${negative / divisor}`;
}

export function DatasetGradeSidebar({
  summary,
}: {
  summary: EvalDatasetResponse;
}) {
  const countsByKey = new Map(
    summary.criteria_counts.map((count) => [count.dimension_key, count]),
  );
  const rows = JUDGMENT_CRITERIA.flatMap((dimension) => {
    const counts = countsByKey.get(dimension.dimensionKey);
    const positive = counts?.good_count ?? 0;
    const negative = counts?.bad_count ?? 0;
    return positive + negative === 0 ? [] : [{ dimension, positive, negative }];
  });

  return (
    <aside className="flex w-full flex-none flex-col gap-4 border-b border-border bg-card p-4 @[780px]/dataset-card:w-[300px] @[780px]/dataset-card:border-b-0 @[780px]/dataset-card:border-r @[780px]/dataset-card:p-5">
      <div>
        <h3 className="text-heading-3 text-foreground">
          Evaluation criteria
        </h3>
        <p className="mt-1.5 text-body-sm text-muted-foreground">
          Evaluations recorded for traces in this dataset.
        </p>
      </div>

      {rows.length === 0 ? (
        <div className="rounded-md border border-dashed border-border px-4 py-5 text-body-sm text-muted-foreground">
          No criteria values recorded yet.
        </div>
      ) : (
        <TooltipProvider delayDuration={300}>
          <div className="flex flex-col gap-5">
            {rows.map(({ dimension, positive, negative }) => (
              <section key={dimension.dimensionKey}>
                <div className="mb-2 flex items-center justify-between gap-3 text-body-sm">
                  <div className="flex min-w-0 items-center gap-1.5 font-semibold text-foreground">
                    {dimension.dimensionLabel}
                    <InfoHint label={`About ${dimension.dimensionLabel}`}>
                      {dimension.goodTooltip}
                    </InfoHint>
                  </div>
                  <span
                    aria-label={`${positive} positive, ${negative} negative`}
                    className="flex-none font-mono text-muted-foreground tabular-nums"
                  >
                    {formatRatio(positive, negative)}
                  </span>
                </div>
                <ProgressBar
                  aria-label={`${dimension.dimensionLabel} positive distribution`}
                  value={positive}
                  max={positive + negative}
                  tone="success"
                  className="h-2 bg-destructive"
                  indicatorClassName="rounded-none"
                />
              </section>
            ))}
          </div>
        </TooltipProvider>
      )}
    </aside>
  );
}
