import { useState, useRef, useEffect } from "react";
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
import { ALL_AGENTS_KEY, ALL_AGENTS_COLOR } from "./use-insights-data";

interface AgentFilterBarProps {
  value: string[];
  onValueChange: (values: string[]) => void;
  allAgentNames: string[];
  colorMap: Record<string, string>;
}

export function AgentFilterBar({ value, onValueChange, allAgentNames, colorMap }: AgentFilterBarProps) {
  const [filterSearch, setFilterSearch] = useState("");
  const [popoverOpen, setPopoverOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (popoverOpen) searchInputRef.current?.focus();
  }, [popoverOpen]);

  function handleChange(values: string[]) {
    const hadAll = value.includes(ALL_AGENTS_KEY);
    const hasAll = values.includes(ALL_AGENTS_KEY);
    if (!hadAll && hasAll) {
      onValueChange([ALL_AGENTS_KEY]);
    } else if (hadAll && values.length > 1) {
      onValueChange(values.filter((v) => v !== ALL_AGENTS_KEY));
    } else {
      onValueChange(values);
    }
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
          {value[0] === ALL_AGENTS_KEY && (
            <span
              className="inline-flex shrink-0 items-center gap-1 rounded-full px-1.5 py-0.5 font-mono text-body-sm text-foreground"
              style={{ background: `color-mix(in srgb, ${ALL_AGENTS_COLOR} 16%, transparent)` }}
            >
              <span className="size-1.5 shrink-0 rounded-full" style={{ background: ALL_AGENTS_COLOR }} />
              All agents
              <Button
                variant="ghost"
                size="icon-xs"
                type="button"
                onPointerDown={(e) => e.stopPropagation()}
                onClick={(e) => { e.stopPropagation(); onValueChange([]); }}
                className="size-3 p-0 text-faint-foreground hover:text-foreground hover:bg-transparent"
              >
                <X className="size-[9px]" />
              </Button>
            </span>
          )}
          {value[0] !== ALL_AGENTS_KEY && value.map((name) => (
            <span
              key={name}
              className="inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 font-mono text-body-sm text-foreground"
              style={{ background: `color-mix(in srgb, ${colorMap[name]} 16%, transparent)` }}
            >
              <span className="size-1.5 shrink-0 rounded-full" style={{ background: colorMap[name] }} />
              {name}
              <Button
                variant="ghost"
                size="icon-xs"
                type="button"
                onPointerDown={(e) => e.stopPropagation()}
                onClick={(e) => { e.stopPropagation(); onValueChange(value.filter((a) => a !== name)); }}
                className="size-3 p-0 text-faint-foreground hover:text-foreground hover:bg-transparent"
              >
                <X className="size-[9px]" />
              </Button>
            </span>
          ))}
          <input
            ref={searchInputRef}
            value={filterSearch}
            onChange={(e) => setFilterSearch(e.target.value)}
            onFocus={() => setPopoverOpen(true)}
            placeholder={value.length === 0 ? `${allAgentNames.length} agents` : ""}
            className="flex-1 min-w-[80px] bg-transparent font-mono text-body-sm text-foreground placeholder:text-faint-foreground outline-none"
          />
        </div>
      </Popover.Anchor>
      <MultiSelectContent>
        <MultiSelectItem value={ALL_AGENTS_KEY} color={ALL_AGENTS_COLOR}>All agents</MultiSelectItem>
        {allAgentNames
          .filter((n) => n.toLowerCase().includes(filterSearch.toLowerCase()))
          .map((name) => (
            <MultiSelectItem key={name} value={name} color={colorMap[name]}>{name}</MultiSelectItem>
          ))}
      </MultiSelectContent>
    </MultiSelect>
  );
}
