import { useState, useDeferredValue, useRef, useEffect } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { Popover as PopoverPrimitive } from "radix-ui";
import { ChevronDownIcon, MagnifyingGlassIcon } from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";
import { TIMEZONE_OPTIONS, type TimezoneOption } from "@/lib/timezone";
import { inputBase, inputFocusVisible } from "./input";

// ── Inner list — mounts fresh each time the popover opens ────────────────────

interface TimezoneListProps {
  value: string;
  items: TimezoneOption[];
  searchRef: React.RefObject<HTMLInputElement | null>;
  onSelect: (tz: string) => void;
}

function TimezoneList({ value, items, searchRef, onSelect }: TimezoneListProps) {
  const scrollRef = useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 32,
    overscan: 10,
  });

  // Focus search and scroll selected item into view on mount.
  useEffect(() => {
    searchRef.current?.focus();
    const idx = items.findIndex((o) => o.value === value);
    if (idx >= 0) virtualizer.scrollToIndex(idx, { align: "center" });
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div ref={scrollRef} className="h-[300px] overflow-y-auto">
      <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
        {virtualizer.getVirtualItems().map((vItem) => {
          const option = items[vItem.index];
          const isSelected = option.value === value;
          return (
            <button
              key={vItem.key}
              type="button"
              onClick={() => onSelect(option.value)}
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                width: "100%",
                height: `${vItem.size}px`,
                transform: `translateY(${vItem.start}px)`,
              }}
              className={cn(
                "flex items-center px-3 text-body-sm text-foreground hover:bg-muted transition-colors text-left",
                isSelected && "bg-muted font-medium",
              )}
            >
              {option.label}
            </button>
          );
        })}
      </div>
    </div>
  );
}

// ── Public component ──────────────────────────────────────────────────────────

interface TimezoneSelectProps {
  value: string;
  onValueChange: (tz: string) => void;
  className?: string;
}

export function TimezoneSelect({ value, onValueChange, className }: TimezoneSelectProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  const deferredSearch = useDeferredValue(search);
  const searchRef = useRef<HTMLInputElement>(null);

  const selected = TIMEZONE_OPTIONS.find((o) => o.value === value);
  const triggerLabel = selected?.label ?? value;

  const filtered: TimezoneOption[] = deferredSearch
    ? TIMEZONE_OPTIONS.filter((o) =>
        o.label.toLowerCase().includes(deferredSearch.toLowerCase()),
      )
    : TIMEZONE_OPTIONS;

  function handleOpenChange(next: boolean) {
    setOpen(next);
    if (!next) setSearch("");
  }

  function handleSelect(tz: string) {
    onValueChange(tz);
    setOpen(false);
  }

  return (
    <PopoverPrimitive.Root open={open} onOpenChange={handleOpenChange}>
      <PopoverPrimitive.Trigger asChild>
        <button
          type="button"
          className={cn(
            "group inline-flex h-9 w-full items-center justify-between gap-1.5 px-3 text-body-sm",
            inputBase,
            inputFocusVisible,
            "data-[state=open]:border-teal-600 data-[state=open]:ring-[3px] data-[state=open]:ring-[var(--input-focus-ring)] dark:data-[state=open]:border-teal-400",
            className,
          )}
        >
          <span className="truncate">{triggerLabel}</span>
          <ChevronDownIcon className="size-3 shrink-0 opacity-50 transition-transform duration-150 group-data-[state=open]:rotate-180" />
        </button>
      </PopoverPrimitive.Trigger>

      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Content
          align="start"
          sideOffset={6}
          className={cn(
            "z-50 w-[320px] overflow-hidden rounded-md border border-border bg-popover shadow-md",
            "data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=top]:slide-in-from-bottom-2",
          )}
        >
          {/* Search */}
          <div className="flex items-center gap-2 border-b border-border px-3 py-2">
            <MagnifyingGlassIcon className="size-3.5 shrink-0 text-muted-foreground" />
            <input
              ref={searchRef}
              type="text"
              placeholder="Search"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="flex-1 bg-transparent text-body-sm outline-none placeholder:text-muted-foreground"
            />
          </div>

          {/* Virtualized list — mounted fresh on each open */}
          <TimezoneList
            value={value}
            items={filtered}
            searchRef={searchRef}
            onSelect={handleSelect}
          />
        </PopoverPrimitive.Content>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  );
}
