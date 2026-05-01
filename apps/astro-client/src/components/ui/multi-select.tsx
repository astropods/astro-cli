import * as React from "react";
import { ChevronDownIcon, CheckIcon } from "@heroicons/react/24/outline";
import { Popover as PopoverPrimitive } from "radix-ui";
import { cn } from "@/lib/utils";
import { inputBase, inputFocusVisible } from "./input";

export interface MultiSelectOption {
  value: string;
  label: string;
  color?: string;
}

// ── Context ──────────────────────────────────────────────────────────────────

interface MultiSelectContextValue {
  value: string[];
  onValueChange: (v: string[]) => void;
}

const MultiSelectContext = React.createContext<MultiSelectContextValue | null>(null);

function useMultiSelect() {
  const ctx = React.useContext(MultiSelectContext);
  if (!ctx) throw new Error("MultiSelect primitives must be used within <MultiSelect>");
  return ctx;
}

// ── Root ─────────────────────────────────────────────────────────────────────

interface MultiSelectRootProps {
  value: string[];
  onValueChange: (v: string[]) => void;
  children: React.ReactNode;
}

function MultiSelect({ value, onValueChange, children }: MultiSelectRootProps) {
  return (
    <MultiSelectContext.Provider value={{ value, onValueChange }}>
      <PopoverPrimitive.Root data-slot="multi-select">
        {children}
      </PopoverPrimitive.Root>
    </MultiSelectContext.Provider>
  );
}

// ── Trigger ───────────────────────────────────────────────────────────────────

function MultiSelectTrigger({ className, children, ...props }: React.ComponentProps<"button">) {
  const { value } = useMultiSelect();
  return (
    <PopoverPrimitive.Trigger asChild>
      <button
        type="button"
        data-slot="multi-select-trigger"
        className={cn(
          "group inline-flex h-9 w-full items-center justify-between gap-1.5 px-3 text-body-sm",
          inputBase,
          inputFocusVisible,
          "bg-transparent data-[state=open]:border-slate-600 data-[state=open]:ring-[3px] data-[state=open]:ring-[var(--input-focus-ring)] dark:data-[state=open]:border-slate-400",
          value.length > 0 && "border-ring",
          className,
        )}
        {...props}
      >
        {children}
        <ChevronDownIcon className="size-3 shrink-0 opacity-50 transition-transform duration-150 group-data-[state=open]:rotate-180" />
      </button>
    </PopoverPrimitive.Trigger>
  );
}

// ── Value ─────────────────────────────────────────────────────────────────────

interface MultiSelectValueProps {
  placeholder?: string;
  options: MultiSelectOption[];
}

function MultiSelectValue({ placeholder = "All", options }: MultiSelectValueProps) {
  const { value } = useMultiSelect();
  const allSelected = value.length === 0 || value.length === options.length;
  const label = allSelected
    ? placeholder
    : value.length === 1
      ? (options.find((o) => o.value === value[0])?.label ?? value[0])
      : `${value.length} selected`;
  return <span className="truncate">{label}</span>;
}

// ── Content ───────────────────────────────────────────────────────────────────

function MultiSelectContent({
  className,
  children,
}: {
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Content
        data-slot="multi-select-content"
        align="start"
        sideOffset={6}
        className={cn(
          "z-50 min-w-[var(--radix-popover-trigger-width)] overflow-hidden rounded-md border border-border bg-popover shadow-md",
          "data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95 data-[side=bottom]:slide-in-from-top-2 data-[side=top]:slide-in-from-bottom-2",
          className,
        )}
      >
        {children}
      </PopoverPrimitive.Content>
    </PopoverPrimitive.Portal>
  );
}

// ── All Item ──────────────────────────────────────────────────────────────────

function MultiSelectAllItem({ children = "All" }: { children?: React.ReactNode }) {
  const { value, onValueChange } = useMultiSelect();
  const allSelected = value.length === 0;
  return (
    <button
      type="button"
      onClick={() => onValueChange([])}
      className="flex w-full items-center gap-2 border-b border-border px-3 py-2 text-sm text-muted-foreground hover:bg-muted transition-colors"
    >
      <span
        className={cn(
          "flex size-3.5 shrink-0 items-center justify-center rounded-xs border transition-colors",
          allSelected ? "border-primary bg-primary text-primary-foreground" : "border-border",
        )}
      >
        {allSelected && <CheckIcon className="size-2.5" />}
      </span>
      {children}
    </button>
  );
}

// ── Item ──────────────────────────────────────────────────────────────────────

interface MultiSelectItemProps {
  value: string;
  color?: string;
  children: React.ReactNode;
  className?: string;
}

function MultiSelectItem({ value, color, children, className }: MultiSelectItemProps) {
  const { value: selected, onValueChange } = useMultiSelect();
  const checked = selected.includes(value);
  const toggle = () =>
    onValueChange(checked ? selected.filter((s) => s !== value) : [...selected, value]);
  return (
    <button
      type="button"
      onClick={toggle}
      className={cn(
        "flex w-full items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-muted transition-colors",
        className,
      )}
    >
      <span
        className={cn(
          "flex size-3.5 shrink-0 items-center justify-center rounded-xs border transition-colors",
          checked ? "border-primary bg-primary text-primary-foreground" : "border-border",
        )}
      >
        {checked && <CheckIcon className="size-2.5" />}
      </span>
      {color && <span className="size-1.5 shrink-0 rounded-full" style={{ background: color }} />}
      {children}
    </button>
  );
}

export {
  MultiSelect,
  MultiSelectTrigger,
  MultiSelectValue,
  MultiSelectContent,
  MultiSelectAllItem,
  MultiSelectItem,
};
