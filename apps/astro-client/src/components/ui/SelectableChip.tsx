import * as React from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export type SelectableChipTone = "primary" | "success" | "destructive";

const TONE_ACTIVE_CLASSES: Record<SelectableChipTone, string> = {
  primary:
    "data-[active]:border-primary/40 data-[active]:bg-primary/10 data-[active]:text-foreground dark:data-[active]:border-primary/50 dark:data-[active]:bg-primary/15 dark:data-[active]:text-foreground",
  success:
    "data-[active]:border-success/50 data-[active]:bg-success/20 data-[active]:text-success",
  destructive:
    "data-[active]:border-destructive/50 data-[active]:bg-destructive/20 data-[active]:text-destructive",
};

export interface SelectableChipProps extends React.ComponentProps<"button"> {
  selected: boolean;
  tone?: SelectableChipTone;
}

/** A rounded, toggleable multi-select pill. Owns the shell, tone, and selected
 *  state; leading/trailing content (dots, counts, icons) is caller-owned. */
export function SelectableChip({
  selected,
  tone = "primary",
  className,
  children,
  ...props
}: SelectableChipProps) {
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      data-active={selected || undefined}
      aria-pressed={selected}
      className={cn(
        "h-8 gap-1.5 rounded-full px-3 text-body-sm font-medium transition-colors",
        TONE_ACTIVE_CLASSES[tone],
        className,
      )}
      {...props}
    >
      {children}
    </Button>
  );
}
