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
        "flex items-center gap-2 rounded border border-border bg-background px-3 py-1.5",
        "focus-within:border-teal-700 focus-within:ring-[2px] focus-within:ring-teal-700 focus-within:ring-offset-2",
        containerClassName
      )}
    >
      <Filter className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
      <input
        type="text"
        className={cn(
          "w-full bg-transparent text-sm text-foreground placeholder:text-muted-foreground outline-none",
          className
        )}
        {...props}
      />
    </div>
  );
}
