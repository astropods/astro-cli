import { type ReactNode } from "react";

import { ACTIVITY_RANGES } from "./ranges";
import { PillToggle, type PillOption, type PillToggleSize } from "./PillToggle";
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
  /** Readout shown inside the track ahead of the chips. See `PillToggle`. */
  leading?: ReactNode;
  size?: PillToggleSize;
  className?: string;
}

export function TimeRangeSelector({
  value,
  ranges = ACTIVITY_RANGES,
  onChange,
  layoutId = "range-pill",
  leading,
  size,
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
      leading={leading}
      size={size}
      className={cn("w-fit", className)}
    />
  );
}
