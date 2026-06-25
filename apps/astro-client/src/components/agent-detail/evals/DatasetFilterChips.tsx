import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export type FilterKey = "good" | "bad";

interface Chip {
  key: FilterKey;
  label: string;
  count: number;
  dotClass: string;
}

export interface DatasetFilterChipsProps {
  selected: Set<FilterKey>;
  counts: { good: number; bad: number };
  onToggle: (key: FilterKey) => void;
}

export function DatasetFilterChips({ selected, counts, onToggle }: DatasetFilterChipsProps) {
  const chips: Chip[] = [
    { key: "good", label: "Good", count: counts.good, dotClass: "bg-success" },
    { key: "bad", label: "Bad", count: counts.bad, dotClass: "bg-destructive" },
  ];

  return (
    <div className="flex flex-wrap items-center gap-2">
      {chips.map((chip) => {
        const active = selected.has(chip.key);
        return (
          <Button
            key={chip.key}
            variant="outline"
            size="sm"
            data-active={active || undefined}
            onClick={() => onToggle(chip.key)}
            className={cn(
              "h-8 gap-1.5 rounded-full px-3 text-body-sm font-medium",
              "data-[active]:border-primary/40 data-[active]:bg-primary/10 data-[active]:text-foreground",
              "dark:data-[active]:border-primary/50 dark:data-[active]:bg-primary/15 dark:data-[active]:text-foreground",
            )}
          >
            <span aria-hidden className={cn("size-1.5 rounded-full", chip.dotClass)} />
            {chip.label}
            <span
              className={cn(
                "inline-flex h-[18px] min-w-[18px] items-center justify-center rounded-full px-1.5 font-mono text-mono-sm",
                active
                  ? "bg-primary/20 text-foreground"
                  : "bg-muted text-muted-foreground",
              )}
            >
              {chip.count.toLocaleString()}
            </span>
            {active && <X aria-hidden className="size-3 opacity-75" />}
          </Button>
        );
      })}
    </div>
  );
}
