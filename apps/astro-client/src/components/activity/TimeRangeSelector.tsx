import { ACTIVITY_RANGES } from "./ranges";
import { PillToggle, type PillOption } from "./PillToggle";

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
    />
  );
}
