import { useEffect, useMemo, useRef, useState } from "react";
import { MagnifyingGlassIcon } from "@heroicons/react/24/outline";
import { X } from "lucide-react";
import { Popover } from "radix-ui";
import {
  MultiSelect,
  MultiSelectContent,
  MultiSelectItem,
} from "@/components/ui/multi-select";
import { Button } from "@/components/ui/button";
import { inputBase, inputFocusWithin } from "@/components/ui/input";
import { cn } from "@/lib/utils";

export interface FilterEntry {
  key: string;
  label: string;
  color: string;
}

interface MultiSelectFilterBarProps {
  value: string[];
  onValueChange: (values: string[]) => void;
  /** Selectable entries — do NOT include the "All" item here, it's pinned via allItem. */
  entries: FilterEntry[];
  /** "All" sentinel pinned at the top of the dropdown. Selecting it clears
   *  other selections; selecting another item drops the All key automatically. */
  allItem: FilterEntry;
  placeholder?: string;
}

export function MultiSelectFilterBar({
  value,
  onValueChange,
  entries,
  allItem,
  placeholder,
}: MultiSelectFilterBarProps) {
  const [filterSearch, setFilterSearch] = useState("");
  const [popoverOpen, setPopoverOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (popoverOpen) searchInputRef.current?.focus();
  }, [popoverOpen]);

  function handleChange(values: string[]) {
    const hadAll = value.includes(allItem.key);
    const hasAll = values.includes(allItem.key);
    if (!hadAll && hasAll) onValueChange([allItem.key]);
    else if (hadAll && values.length > 1) onValueChange(values.filter((v) => v !== allItem.key));
    else onValueChange(values);
  }

  // Looked up on every selected chip render — O(1) lookup beats repeated find().
  const entriesByKey = useMemo(() => new Map(entries.map((e) => [e.key, e])), [entries]);

  function labelFor(key: string): string {
    if (key === allItem.key) return allItem.label;
    return entriesByKey.get(key)?.label ?? key;
  }
  function colorFor(key: string): string {
    if (key === allItem.key) return allItem.color;
    return entriesByKey.get(key)?.color ?? "var(--color-muted-foreground)";
  }

  return (
    <MultiSelect
      value={value}
      onValueChange={handleChange}
      open={popoverOpen}
      onOpenChange={(v) => { setPopoverOpen(v); if (!v) setFilterSearch(""); }}
    >
      <Popover.Anchor asChild>
        <div
          role="combobox"
          aria-expanded={popoverOpen}
          className={cn(
            inputBase,
            inputFocusWithin,
            "inline-flex flex-wrap min-w-[calc(50%-0.375rem)] max-w-full cursor-text items-center gap-2 py-1 bg-transparent",
          )}
          onPointerDown={(e) => {
            if (!(e.target instanceof HTMLInputElement)) {
              e.preventDefault();
              searchInputRef.current?.focus();
            }
          }}
        >
          <MagnifyingGlassIcon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          {value.map((key) => {
            const color = colorFor(key);
            return (
              <span
                key={key}
                className="inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 font-mono text-body-sm text-foreground"
                style={{ background: `color-mix(in srgb, ${color} 16%, transparent)` }}
              >
                <span className="size-1.5 shrink-0 rounded-full" style={{ background: color }} />
                {labelFor(key)}
                <Button
                  variant="ghost"
                  size="icon-xs"
                  type="button"
                  onPointerDown={(e) => e.stopPropagation()}
                  onClick={(e) => { e.stopPropagation(); onValueChange(value.filter((k) => k !== key)); }}
                  className="size-3 p-0 text-faint-foreground hover:text-foreground hover:bg-transparent"
                >
                  <X className="size-[9px]" />
                </Button>
              </span>
            );
          })}
          <input
            ref={searchInputRef}
            value={filterSearch}
            onChange={(e) => setFilterSearch(e.target.value)}
            onFocus={() => setPopoverOpen(true)}
            placeholder={value.length === 0 ? (placeholder ?? "") : ""}
            className="flex-1 min-w-[80px] bg-transparent font-mono text-body-sm text-foreground placeholder:text-faint-foreground outline-none"
          />
        </div>
      </Popover.Anchor>
      <MultiSelectContent>
        <MultiSelectItem value={allItem.key} color={allItem.color}>{allItem.label}</MultiSelectItem>
        {entries
          .filter((e) => e.label.toLowerCase().includes(filterSearch.toLowerCase()))
          .map((e) => (
            <MultiSelectItem key={e.key} value={e.key} color={e.color}>{e.label}</MultiSelectItem>
          ))}
      </MultiSelectContent>
    </MultiSelect>
  );
}
