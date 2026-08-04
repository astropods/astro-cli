import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface FilteredEmptyStateProps {
  message: string;
  onClear: () => void;
  className?: string;
}

export function FilteredEmptyState({ message, onClear, className }: FilteredEmptyStateProps) {
  return (
    <div className={cn(
      "flex flex-col items-center justify-center gap-4 rounded-lg border border-dashed border-border px-6 py-16 text-center",
      className,
    )}>
      <p className="text-body-sm text-muted-foreground">{message}</p>
      <Button variant="outline" size="sm" onClick={onClear}>Clear filters</Button>
    </div>
  );
}
