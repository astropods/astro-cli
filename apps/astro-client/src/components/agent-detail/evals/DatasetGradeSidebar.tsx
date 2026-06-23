import type { EvalDatasetResponse } from "@/lib/api";
import { DatasetGrade } from "./DatasetGrade";

function SectionLabel({ children }: { children: React.ReactNode }) {
  return (
    <div className="font-mono text-label uppercase text-faint-foreground">
      {children}
    </div>
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
    <aside className="flex w-[268px] flex-none flex-col gap-6 border-r border-border bg-card p-5 pt-5.5 dark:bg-surface">
      <DatasetGrade
        grade={grade}
        variant="label"
        itemCount={item_count}
        nextGrade={next_grade}
        nextGradeProgress={next_grade_progress}
      />

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
        <div className="mt-2.5 flex gap-4 text-body-sm text-muted-foreground">
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
