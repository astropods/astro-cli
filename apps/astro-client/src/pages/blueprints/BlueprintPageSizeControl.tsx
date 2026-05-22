import {
  BLUEPRINT_PAGE_SIZE_OPTIONS,
  type BlueprintPageSize,
} from '@/lib/blueprint-list-params';
import { persistBlueprintPageSize } from '@/lib/blueprint-page-size-preference';
import { TimeRangeSelector } from '@/components/activity/TimeRangeSelector';

const PAGE_SIZE_RANGES = BLUEPRINT_PAGE_SIZE_OPTIONS.map((size) => ({
  key: String(size),
  label: String(size),
  ariaLabel: `${size} per page`,
}));

export function BlueprintPageSizeControl({
  value,
  onChange,
}: {
  value: BlueprintPageSize;
  onChange: (size: BlueprintPageSize) => void;
}) {
  return (
    <div className="flex shrink-0 items-center gap-2">
      <span className="text-body-sm text-muted-foreground whitespace-nowrap">Per page</span>
      <TimeRangeSelector
        value={String(value)}
        ranges={PAGE_SIZE_RANGES}
        onChange={(next) => {
          const size = Number(next) as BlueprintPageSize;
          persistBlueprintPageSize(size);
          onChange(size);
        }}
        layoutId="blueprint-page-size-pill"
      />
    </div>
  );
}
