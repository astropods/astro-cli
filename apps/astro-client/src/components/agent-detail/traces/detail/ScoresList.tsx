import type { TraceScore } from "@/lib/api";

export interface ScoresListProps {
  scores: TraceScore[];
}

function formatScoreValue(score: TraceScore): string {
  if (score.string_value) return score.string_value;
  if (score.data_type === "boolean") return score.value ? "true" : "false";
  if (Number.isFinite(score.value)) {
    return Number.isInteger(score.value)
      ? String(score.value)
      : score.value.toFixed(3);
  }
  return "—";
}

export function ScoresList({ scores }: ScoresListProps) {
  if (scores.length === 0) return null;

  return (
    <section className="flex flex-col gap-2 rounded-md border border-border/40 px-4 py-3">
      <h4 className="text-body-sm font-medium text-foreground">Scores</h4>
      <ul className="flex flex-col gap-1.5">
        {scores.map((s) => (
          <li
            key={s.id}
            className="flex items-baseline gap-2 text-body-sm"
            title={s.comment || undefined}
          >
            <span className="font-mono text-mono-sm text-muted-foreground">
              {s.name}
            </span>
            <span className="font-mono text-foreground">
              {formatScoreValue(s)}
            </span>
            {s.source && (
              <span className="ml-auto text-mono-sm text-muted-foreground/60">
                {s.source}
              </span>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
