import { HelpCircle, Info, Lightbulb, type LucideIcon } from "lucide-react";
import { type ReactNode, useMemo, useState } from "react";
import { Card } from "@/components/ui/card";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { EvalDatasetItemsVerdict, EvalDatasetResponse } from "@/lib/api";
import { cn } from "@/lib/utils";
import { DatasetGrade } from "./DatasetGrade";
import {
  criterionLabel,
  criterionTooltip,
  JUDGMENT_CRITERIA,
} from "../judgment-criteria";

const TARGET_SCORED_CASES = 100;
const TARGET_BAD_SHARE = 0.1;
const HIGH_BAD_SHARE = 0.25;

function gradeGuidance(summary: EvalDatasetResponse) {
  const scoredCount = summary.good_count + summary.bad_count;
  const badShare = scoredCount > 0 ? summary.bad_count / scoredCount : 0;
  const badPct = Math.round(badShare * 100);

  switch (true) {
    case scoredCount === 0:
      return {
        title: "Start grading",
        body:
          "Label recent traces as good or bad. These labels determine how reliable this dataset is.",
      };
    case scoredCount < TARGET_SCORED_CASES:
      return {
        title: "Grade more cases",
        body:
          summary.cases_to_next_grade != null && summary.next_grade
            ? `Label ${summary.cases_to_next_grade.toLocaleString()} or more traces to raise this grade to a ${summary.next_grade}. Include some bad cases to keep the score reliable.`
            : "More labels make the dataset score more reliable. Make sure to include some bad cases.",
      };
    case badShare < TARGET_BAD_SHARE:
      return {
        title: "Add failure cases",
        body: `Only ${badPct}% of traces are labeled bad. Add failure cases so your dataset captures how the agent actually fails.`,
      };
    case badShare > HIGH_BAD_SHARE:
      return {
        title: "Reduce noise",
        body: `${badPct}% of traces are labeled bad. Add good responses or remove bad labels that don't reflect real failures.`,
      };
    case summary.grade.toUpperCase() === "A":
      return {
        title: "Dataset looks healthy",
        body:
          "This dataset is a reliable signal. Keep grading as the agent's behavior changes.",
      };
    default:
      return {
        title: "Improve your dataset",
        body:
          "Continue grading traces to increase your dataset's reliability.",
      };
  }
}

function GradeGuidanceCard({ summary }: { summary: EvalDatasetResponse }) {
  const guidance = gradeGuidance(summary);

  return (
    <Card className="rounded-md border-primary/35 bg-primary/10 p-3 text-foreground shadow-sm">
      <div className="flex items-center gap-2">
        <Lightbulb
          aria-hidden
          className="size-3.5 flex-none text-foreground-accent"
        />
        <div className="min-w-0 text-body-sm font-semibold leading-5 text-foreground">
          {guidance.title}
        </div>
      </div>
      <p className="mt-1.5 text-body-sm leading-5 text-foreground">
        {guidance.body}
      </p>
    </Card>
  );
}

/** An info/help icon that reveals `children` on hover. */
function InfoHint({
  icon: Icon,
  label,
  children,
}: {
  icon: LucideIcon;
  label: string;
  children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          tabIndex={0}
          aria-label={label}
          className="inline-flex flex-none cursor-help text-faint-foreground transition-colors hover:text-muted-foreground"
        >
          <Icon aria-hidden className="size-3.5" />
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-xs">{children}</TooltipContent>
    </Tooltip>
  );
}

function GradeHeader() {
  return (
    <div className="flex min-w-0 items-center gap-1.5">
      <span className="font-mono text-label uppercase text-faint-foreground">
        Baseline grade
      </span>
      <TooltipProvider delayDuration={300}>
        <InfoHint icon={HelpCircle} label="How the grade is computed">
          A rough read on this eval set's strength, from how many examples
          you've labeled and how balanced good vs. bad are. Label more traces
          to raise it.
        </InfoHint>
      </TooltipProvider>
    </div>
  );
}

interface Reason {
  dimensionKey: string;
  label: string;
  tooltip: string;
  count: number;
}

/** Builds the good and bad reason lists in a single pass over the criteria,
 *  each sorted by count descending. */
function buildReasons(
  summary: EvalDatasetResponse,
): Record<EvalDatasetItemsVerdict, Reason[]> {
  const reasons: Record<EvalDatasetItemsVerdict, Reason[]> = { good: [], bad: [] };

  for (const dim of JUDGMENT_CRITERIA) {
    const count = summary.criteria_counts.find(
      (c) => c.dimension_key === dim.dimensionKey,
    );
    if (!count) continue;
    for (const verdict of ["good", "bad"] as const) {
      const value = count[`${verdict}_count`];
      if (value <= 0) continue;
      reasons[verdict].push({
        dimensionKey: dim.dimensionKey,
        label: criterionLabel(dim, verdict),
        tooltip: criterionTooltip(dim, verdict),
        count: value,
      });
    }
  }

  reasons.good.sort((a, b) => b.count - a.count);
  reasons.bad.sort((a, b) => b.count - a.count);
  return reasons;
}

function ReasonsSection({ summary }: { summary: EvalDatasetResponse }) {
  const reasons = useMemo(() => buildReasons(summary), [summary]);
  const [verdict, setVerdict] = useState<EvalDatasetItemsVerdict>(
    reasons.bad.length > 0 ? "bad" : "good",
  );

  if (reasons.good.length === 0 && reasons.bad.length === 0) return null;

  const shown = reasons[verdict];

  return (
    <div>
      <div className="mb-2 flex items-center justify-between gap-2">
        <div className="font-mono text-label uppercase text-faint-foreground">
          Reasons
        </div>
        <ToggleGroup
          type="single"
          variant="word"
          value={verdict}
          onValueChange={(v) => setVerdict(v as EvalDatasetItemsVerdict)}
          indicatorClassName={cn(
            "border-transparent",
            verdict === "good" ? "bg-success/10" : "bg-destructive/10",
          )}
        >
          <ToggleGroupItem
            value="good"
            className="px-2.5 py-1 text-body-sm data-[state=on]:text-success"
          >
            Good
          </ToggleGroupItem>
          <ToggleGroupItem
            value="bad"
            className="px-2.5 py-1 text-body-sm data-[state=on]:text-destructive"
          >
            Bad
          </ToggleGroupItem>
        </ToggleGroup>
      </div>
      {shown.length === 0 ? (
        <p className="py-2.5 text-body-sm text-muted-foreground">
          No {verdict} reasons labeled yet.
        </p>
      ) : (
        <TooltipProvider delayDuration={300}>
          <ul>
            {shown.map(({ dimensionKey, label, tooltip, count }) => (
              <li
                key={dimensionKey}
                className="flex items-center justify-between gap-2 border-b border-border py-2.5"
              >
                <span className="flex min-w-0 items-center gap-1.5 text-body-sm text-foreground">
                  <span className="truncate">{label}</span>
                  <InfoHint icon={Info} label={`About ${label}`}>
                    {tooltip}
                  </InfoHint>
                </span>
                <span className="flex-none font-mono text-body-sm text-muted-foreground">
                  {count.toLocaleString()}
                </span>
              </li>
            ))}
          </ul>
        </TooltipProvider>
      )}
    </div>
  );
}

export function DatasetGradeSidebar({
  summary,
}: {
  summary: EvalDatasetResponse;
}) {
  const { grade, next_grade, next_grade_progress } = summary;

  return (
    <aside className="flex w-full flex-none flex-col gap-5 border-b border-border bg-card p-4 @[780px]/dataset-card:w-[268px] @[780px]/dataset-card:border-b-0 @[780px]/dataset-card:border-r @[780px]/dataset-card:p-5 @[780px]/dataset-card:pt-5.5">
      <GradeHeader />

      <DatasetGrade
        grade={grade}
        variant="ring"
        nextGrade={next_grade}
        progress={next_grade_progress}
      />

      <GradeGuidanceCard summary={summary} />

      <ReasonsSection summary={summary} />
    </aside>
  );
}
