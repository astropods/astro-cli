import { Filter } from "lucide-react";
import { cn } from "@/lib/utils";

export interface FilterInputProps
  extends Omit<React.ComponentProps<"input">, "type"> {
  containerClassName?: string;
}

export function FilterInput({
  className,
  containerClassName,
  ...props
}: FilterInputProps) {
  return (
    <div
      className={cn(
        "flex h-11 items-center gap-2 rounded-sm border border-border bg-[var(--input-background)] px-3.5 has-disabled:pointer-events-none has-disabled:cursor-not-allowed has-disabled:opacity-50",
        "focus-within:border-teal-600 focus-within:ring-[3px] focus-within:ring-[var(--input-focus-ring)] dark:focus-within:border-teal-400",
        containerClassName
      )}
    >
      <Filter className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
      <input
        type="text"
        className={cn(
          "w-full bg-transparent text-body text-foreground placeholder:text-muted-foreground outline-none",
          className
        )}
        {...props}
      />
    </div>
  );
}
