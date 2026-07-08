import { X } from "lucide-react";
import { SelectableChip } from "@/components/ui/SelectableChip";
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
          <SelectableChip
            key={chip.key}
            selected={active}
            tone="primary"
            onClick={() => onToggle(chip.key)}
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
          </SelectableChip>
        );
      })}
    </div>
  );
}
