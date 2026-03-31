import { cn } from "@/lib/utils";

interface MetricCellProps {
  label: string;
  value: React.ReactNode;
  className?: string;
}

export function MetricCell({ label, value, className }: MetricCellProps) {
  return (
    <div className={cn("flex flex-col gap-0.5", className)}>
      <span className="text-body-sm text-muted-foreground">{label}</span>
      <span className="text-mono-sm text-foreground tabular-nums">{value}</span>
    </div>
  );
}
