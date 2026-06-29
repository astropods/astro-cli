import { cn } from "@/lib/utils";

type DatasetGradeTone = "success" | "warning" | "destructive" | "muted";

type DatasetGradeProps = {
  grade: string;
  variant?: "badge" | "label";
  itemCount?: number;
  nextGrade?: string;
  nextGradeProgress?: number;
  casesToNextGrade?: number | null;
  className?: string;
};

const GRADE_CLASS: Record<
  DatasetGradeTone,
  {
    foreground: string;
    badgeBackground: string;
    badgeBorder: string;
    progress: string;
  }
> = {
  success: {
    foreground: "text-success",
    badgeBackground: "bg-success/10",
    badgeBorder: "border-success/30",
    progress: "bg-success",
  },
  warning: {
    foreground: "text-warning",
    badgeBackground: "bg-warning/10",
    badgeBorder: "border-warning/30",
    progress: "bg-warning",
  },
  destructive: {
    foreground: "text-destructive",
    badgeBackground: "bg-destructive/10",
    badgeBorder: "border-destructive/30",
    progress: "bg-destructive",
  },
  muted: {
    foreground: "text-muted-foreground",
    badgeBackground: "bg-muted",
    badgeBorder: "border-border",
    progress: "bg-muted-foreground",
  },
};

const GRADE_HEADLINE: Record<string, string> = {
  A: "Dataset looks healthy",
  B: "Improve your dataset",
  C: "Needs more coverage",
  D: "Needs more coverage",
  F: "Needs more coverage",
};

function datasetGradeTone(grade: string): DatasetGradeTone {
  switch (grade.toUpperCase()) {
    case "A":
    case "B":
      return "success";
    case "C":
      return "warning";
    case "D":
    case "F":
      return "destructive";
    default:
      return "muted";
  }
}

function datasetGradeHeadline(grade: string, itemCount: number): string {
  if (itemCount === 0) return "";
  return GRADE_HEADLINE[grade.toUpperCase()] ?? "";
}

function clampProgress(progress: number): number {
  return Math.max(0, Math.min(1, progress));
}

function nextGradeLabel(
  casesToNextGrade: number | null | undefined,
  nextGrade: string,
): string {
  if (casesToNextGrade == null) return `Keep grading to ${nextGrade}`;
  return `at least ${casesToNextGrade.toLocaleString()} mixed labels to ${nextGrade}`;
}

export function DatasetGrade({
  grade,
  variant = "badge",
  itemCount = 0,
  nextGrade = "",
  nextGradeProgress = 0,
  casesToNextGrade,
  className,
}: DatasetGradeProps) {
  const toneClass = GRADE_CLASS[datasetGradeTone(grade)];

  if (variant === "label") {
    const headline = datasetGradeHeadline(grade, itemCount);
    const progress = clampProgress(nextGradeProgress);
    const showProgress = itemCount > 0 && nextGrade !== "";

    return (
      <div className={className}>
        <div className="font-mono text-label uppercase text-faint-foreground">
          Dataset reliability
        </div>
        <div
          className={cn(
            "mt-2 text-[48px] font-semibold leading-none tracking-normal",
            toneClass.foreground,
          )}
          aria-label={`Grade ${grade}`}
        >
          {grade}
        </div>
        {headline && (
          <div
            className={cn("mt-2.5 text-body font-semibold", toneClass.foreground)}
          >
            {headline}
          </div>
        )}
        {showProgress && (
          <div className="mt-3 flex items-center gap-2">
            <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
              <div
                className={cn(
                  "h-full rounded-full transition-[width] duration-300",
                  toneClass.progress,
                )}
                style={{ width: `${progress * 100}%` }}
              />
            </div>
            <span className="whitespace-nowrap font-mono text-mono-sm text-muted-foreground">
              {nextGradeLabel(casesToNextGrade, nextGrade)}
            </span>
          </div>
        )}
      </div>
    );
  }

  return (
    <span
      className={cn(
        "inline-flex size-7 items-center justify-center rounded-sm border font-mono text-body-sm font-bold",
        toneClass.foreground,
        toneClass.badgeBackground,
        toneClass.badgeBorder,
        className,
      )}
      aria-label={`Grade ${grade}`}
    >
      {grade}
    </span>
  );
}
