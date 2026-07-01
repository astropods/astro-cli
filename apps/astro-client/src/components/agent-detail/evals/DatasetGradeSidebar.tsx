import { HelpCircle, Lightbulb } from "lucide-react";
import { Card } from "@/components/ui/card";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { EvalDatasetResponse } from "@/lib/api";
import { DatasetGrade } from "./DatasetGrade";

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
          className="size-3.5 flex-none text-primary"
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

function GradeHeader() {
  return (
    <div className="flex min-w-0 items-center gap-1.5">
      <span className="font-mono text-label uppercase text-faint-foreground">
        Baseline grade
      </span>
      <TooltipProvider delayDuration={300}>
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              tabIndex={0}
              aria-label="How the grade is computed"
              className="inline-flex cursor-help text-faint-foreground transition-colors hover:text-muted-foreground"
            >
              <HelpCircle aria-hidden className="size-3.5" />
            </span>
          </TooltipTrigger>
          <TooltipContent className="max-w-xs">
            A rough read on this eval set's strength, from how many examples
            you've labeled and how balanced good vs. bad are. Label more
            traces to raise it.
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
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
    <aside className="flex w-full flex-none flex-col gap-5 border-b border-border bg-card p-4 dark:bg-surface @[780px]/dataset-card:w-[268px] @[780px]/dataset-card:border-b-0 @[780px]/dataset-card:border-r @[780px]/dataset-card:p-5 @[780px]/dataset-card:pt-5.5">
      <GradeHeader />

      <DatasetGrade
        grade={grade}
        variant="ring"
        nextGrade={next_grade}
        progress={next_grade_progress}
      />

      <GradeGuidanceCard summary={summary} />
    </aside>
  );
}
