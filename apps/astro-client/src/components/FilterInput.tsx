import { MagnifyingGlassIcon } from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";
import { inputBase, inputFocusWithin } from "./ui/input";

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
        "flex h-9 items-center gap-2 has-disabled:pointer-events-none has-disabled:cursor-not-allowed has-disabled:opacity-50",
        inputBase,
        inputFocusWithin,
        containerClassName
      )}
    >
      <MagnifyingGlassIcon className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
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
