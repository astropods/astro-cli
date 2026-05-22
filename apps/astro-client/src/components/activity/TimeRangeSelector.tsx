import { motion } from "motion/react";
import { cn } from "@/lib/utils";
import { ACTIVITY_RANGES } from "./ranges";

interface RangeOption {
  key: string;
  label: string;
  ariaLabel?: string;
}

interface TimeRangeSelectorProps {
  value: string;
  ranges?: RangeOption[];
  onChange: (r: string) => void;
  layoutId?: string;
}

export function TimeRangeSelector({
  value,
  ranges = ACTIVITY_RANGES,
  onChange,
  layoutId = "range-pill",
}: TimeRangeSelectorProps) {
  return (
    <div className="flex items-center rounded-md border border-border bg-surface/60 p-0.5">
      {ranges.map(({ key, label, ariaLabel }) => (
        <button
          key={key}
          type="button"
          onClick={() => onChange(key)}
          aria-label={ariaLabel}
          aria-pressed={key === value}
          className={cn(
            "relative rounded-[10px] px-3 py-1 text-mono-sm font-medium transition-colors",
            key === value
              ? "text-foreground"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          {key === value && (
            <motion.div
              layoutId={layoutId}
              className="absolute inset-0 rounded-[10px] bg-primary/15 dark:bg-white/10"
              transition={{ type: "spring", bounce: 0.15, duration: 0.4 }}
            />
          )}
          <span className="relative">{label}</span>
        </button>
      ))}
    </div>
  );
}
