import { Link } from "react-router";
import { cn } from "@/lib/utils";

export interface UsageCardProps {
  label: string;
  value: number;
  quota: number | undefined;
  unit: string;
  link?: { href: string; label: string };
  loading?: boolean;
  className?: string;
}

export function UsageCard({ label, value, quota, unit, link, loading, className }: UsageCardProps) {
  const pct = quota ? Math.min((value / quota) * 100, 100) : 0;
  const isEmpty = value === 0;

  return (
    <div className={cn("rounded-[10px] border border-border bg-surface p-[12px_14px]", className)}>
      <span className="mb-2 block font-mono text-label uppercase tracking-[0.07em] text-faint-foreground">
        {label}
      </span>
      {loading ? (
        <div className="flex flex-col gap-2 animate-pulse">
          <div className="h-6 w-1/2 rounded bg-muted" />
          <div className="h-1.5 w-full rounded-full bg-muted" />
          <div className="h-3 w-3/5 rounded bg-muted" />
        </div>
      ) : (
        <>
          <div className="flex items-baseline gap-1.5">
            <span className="font-sans text-heading-2 font-bold text-foreground">
              {value.toFixed(2)}
            </span>
            <span className="font-sans text-body-sm text-muted-foreground">
              {unit}
            </span>
          </div>
          <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-muted">
            <div
              className="h-full rounded-full bg-primary transition-[width] duration-300"
              style={{ width: `${pct}%` }}
            />
          </div>
          <div className="mt-1.5 flex items-center justify-between">
            <span className={cn("font-mono text-label", isEmpty ? "text-faint-foreground" : "text-muted-foreground")}>
              {value.toFixed(1)} / {quota ?? "—"} used
            </span>
            {link && (
              <Link
                to={link.href}
                className="font-sans text-body-sm text-primary hover:text-primary/80 transition-colors"
              >
                {link.label}
              </Link>
            )}
          </div>
        </>
      )}
    </div>
  );
}
