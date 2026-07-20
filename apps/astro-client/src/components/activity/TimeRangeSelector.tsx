import { ACTIVITY_RANGES } from "./ranges";
import { PillToggle, type PillOption } from "./PillToggle";
import { cn } from "@/lib/utils";

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
  className?: string;
}

export function TimeRangeSelector({
  value,
  ranges = ACTIVITY_RANGES,
  onChange,
  layoutId = "range-pill",
  className,
}: TimeRangeSelectorProps) {
  const options: PillOption<string>[] = ranges.map((r) => ({
    key: r.key,
    label: r.label,
    ariaLabel: r.ariaLabel,
  }));
  return (
    <PillToggle
      value={value}
      options={options}
      onChange={onChange}
      layoutId={layoutId}
      className={cn("w-fit", className)}
    />
  );
}
