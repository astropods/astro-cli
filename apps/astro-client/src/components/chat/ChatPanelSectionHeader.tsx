import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

export function ChatPanelSectionHeader({
  label,
  icon: Icon,
  count,
  className,
}: {
  label: string;
  icon?: LucideIcon;
  count?: string | number;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "flex min-w-0 items-center gap-2 text-body font-semibold tracking-normal text-foreground",
        className,
      )}
    >
      {Icon ? (
        <Icon
          aria-hidden="true"
          className="size-3.5 shrink-0 text-muted-foreground"
        />
      ) : null}
      {label}
      {count !== undefined ? (
        <span className="inline-flex items-center font-mono text-mono-sm font-normal leading-none text-muted-foreground">
          {count}
        </span>
      ) : null}
    </span>
  );
}
