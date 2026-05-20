import {
  BLUEPRINT_PAGE_SIZE_OPTIONS,
  type BlueprintPageSize,
} from '@/lib/blueprint-list-params';
import { persistBlueprintPageSize } from '@/lib/blueprint-page-size-preference';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';

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
      <ToggleGroup
        type="single"
        variant="word"
        value={String(value)}
        onValueChange={(next) => {
          if (!next) return;
          const size = Number(next) as BlueprintPageSize;
          persistBlueprintPageSize(size);
          onChange(size);
        }}
        aria-label="Blueprints per page"
        className="shrink-0 [&_button]:min-w-[2.25rem] [&_button]:cursor-pointer [&_button]:px-2.5 [&_button]:text-body-sm"
      >
        {BLUEPRINT_PAGE_SIZE_OPTIONS.map((size) => (
          <ToggleGroupItem key={size} value={String(size)} aria-label={`${size} per page`}>
            {size}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  );
}
