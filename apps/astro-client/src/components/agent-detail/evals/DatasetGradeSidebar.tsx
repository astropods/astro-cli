import { Lightbulb } from "lucide-react";
import { Card } from "@/components/ui/card";
import type { EvalDatasetResponse } from "@/lib/api";
import { DatasetGrade } from "./DatasetGrade";

const TARGET_SCORED_CASES = 100;
const TARGET_BAD_SHARE = 0.1;
const HIGH_BAD_SHARE = 0.25;

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div className="font-mono text-label uppercase text-faint-foreground">
      {children}
    </div>
  );
}

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
          "More labels make the dataset score more reliable. Make sure to include some bad cases.",
      };
    case badShare < TARGET_BAD_SHARE:
      return {
        title: "Add failure cases",
        body: `Only ${badPct}% of traces are labeled bad. Add failure cases so your dataset captures how the agent actually fails.`,
      };
    case badShare > HIGH_BAD_SHARE:
      return {
        title: "Reduce noise",
        body: `${badPct}% of traces are labeled bad. Add good examples or remove bad labels that don't reflect real failures.`,
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
    <Card className="rounded-md border-primary/35 bg-primary/10 p-4 text-foreground shadow-sm">
      <div className="flex items-start gap-3">
        <Lightbulb
          aria-hidden
          className="mt-0.5 size-4 flex-none text-primary"
        />
        <div className="min-w-0">
          <div className="text-body-sm font-semibold text-foreground">
            {guidance.title}
          </div>
          <p className="mt-2 text-body-sm leading-6 text-foreground">
            {guidance.body}
          </p>
        </div>
      </div>
    </Card>
  );
}

export function DatasetGradeSidebar({
  summary,
}: {
  summary: EvalDatasetResponse;
}) {
  const {
    item_count,
    good_count,
    bad_count,
    grade,
    next_grade,
    next_grade_progress,
  } = summary;
  const totalJudged = good_count + bad_count;
  const goodPct = totalJudged > 0 ? (good_count / totalJudged) * 100 : 0;
  const badPct = totalJudged > 0 ? (bad_count / totalJudged) * 100 : 0;

  return (
    <aside className="flex w-full flex-none flex-col gap-5 border-b border-border bg-card p-4 dark:bg-surface @[780px]/dataset-card:w-[268px] @[780px]/dataset-card:border-b-0 @[780px]/dataset-card:border-r @[780px]/dataset-card:p-5 @[780px]/dataset-card:pt-5.5">
      <DatasetGrade
        grade={grade}
        variant="label"
        itemCount={item_count}
        nextGrade={next_grade}
        nextGradeProgress={next_grade_progress}
      />
      <GradeGuidanceCard summary={summary} />

      <div>
        <SectionLabel>Composition · {item_count.toLocaleString()}</SectionLabel>
        <div className="mt-2.5 flex h-2 overflow-hidden rounded-full bg-muted">
          {good_count > 0 && (
            <div className="bg-success" style={{ width: `${goodPct}%` }} />
          )}
          {bad_count > 0 && (
            <div className="bg-destructive" style={{ width: `${badPct}%` }} />
          )}
        </div>
        <div className="mt-2.5 flex flex-wrap gap-x-4 gap-y-1.5 text-body-sm text-muted-foreground">
          <span className="inline-flex items-center gap-1.5">
            <span className="size-2 rounded-[2px] bg-success" aria-hidden />
            {good_count.toLocaleString()} good
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="size-2 rounded-[2px] bg-destructive" aria-hidden />
            {bad_count.toLocaleString()} bad
          </span>
        </div>
      </div>
    </aside>
  );
}
