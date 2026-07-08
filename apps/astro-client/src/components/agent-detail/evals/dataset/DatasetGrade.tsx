import { cn } from "@/lib/utils";

type DatasetGradeTone = "success" | "warning" | "destructive" | "muted";

type DatasetGradeProps = {
  grade: string;
  variant?: "badge" | "ring";
  nextGrade?: string;
  progress?: number;
  className?: string;
};

const GRADE_CLASS: Record<
  DatasetGradeTone,
  {
    foreground: string;
    badgeBackground: string;
    badgeBorder: string;
  }
> = {
  success: {
    foreground: "text-success",
    badgeBackground: "bg-success/10",
    badgeBorder: "border-success/30",
  },
  warning: {
    foreground: "text-warning",
    badgeBackground: "bg-warning/10",
    badgeBorder: "border-warning/30",
  },
  destructive: {
    foreground: "text-destructive",
    badgeBackground: "bg-destructive/10",
    badgeBorder: "border-destructive/30",
  },
  muted: {
    foreground: "text-muted-foreground",
    badgeBackground: "bg-muted",
    badgeBorder: "border-border",
  },
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

const RING_RADIUS = 44;
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS;

function GradeRing({
  grade,
  nextGrade,
  progress,
  className,
}: {
  grade: string;
  nextGrade: string;
  progress: number;
  className?: string;
}) {
  const toneClass = GRADE_CLASS[datasetGradeTone(grade)];
  const clamped = Math.max(0, Math.min(1, progress));
  const dashOffset = RING_CIRCUMFERENCE * (1 - clamped);
  const caption = nextGrade
    ? `${Math.round(clamped * 100)}% to ${nextGrade}`
    : grade.toUpperCase() === "A"
      ? "Top grade"
      : null;

  return (
    <div
      className={cn(
        "relative mx-auto flex size-[168px] items-center justify-center",
        className,
      )}
    >
      <svg
        viewBox="0 0 100 100"
        className="size-full -rotate-90"
        aria-hidden
      >
        <circle
          cx="50"
          cy="50"
          r={RING_RADIUS}
          fill="none"
          strokeWidth="8"
          className="stroke-muted"
        />
        <circle
          cx="50"
          cy="50"
          r={RING_RADIUS}
          fill="none"
          strokeWidth="8"
          strokeLinecap="round"
          strokeDasharray={RING_CIRCUMFERENCE}
          strokeDashoffset={dashOffset}
          className={cn(
            "stroke-current transition-[stroke-dashoffset] duration-500",
            toneClass.foreground,
          )}
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <span
          className={cn(
            "text-[44px] font-semibold leading-none tracking-normal",
            toneClass.foreground,
          )}
          aria-label={`Grade ${grade}`}
        >
          {grade}
        </span>
        {caption && (
          <span className="mt-1.5 font-mono text-label text-muted-foreground">
            {caption}
          </span>
        )}
      </div>
    </div>
  );
}

export function DatasetGrade({
  grade,
  variant = "badge",
  nextGrade = "",
  progress = 0,
  className,
}: DatasetGradeProps) {
  const toneClass = GRADE_CLASS[datasetGradeTone(grade)];

  if (variant === "ring") {
    return (
      <GradeRing
        grade={grade}
        nextGrade={nextGrade}
        progress={progress}
        className={className}
      />
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
