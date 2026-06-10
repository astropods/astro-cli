import type { ReactNode } from "react";
import { Popover, PopoverTrigger, PopoverContent } from "@/components/ui/popover";
import { cn } from "@/lib/utils";

export interface SidebarSectionProps {
  title: string;
  badge?: ReactNode;
  badgeTooltip?: string;
  trailing?: ReactNode;
  children: ReactNode;
  className?: string;
  headerClassName?: string;
  bodyClassName?: string;
}

export function SidebarSection({
  title,
  badge,
  badgeTooltip,
  trailing,
  children,
  className,
  headerClassName,
  bodyClassName,
}: SidebarSectionProps) {
  return (
    <section className={`overflow-hidden rounded-[4px] border border-border-strong bg-surface ${className ?? ""}`}>
      <header className={cn("flex items-center gap-2 border-b border-border-strong bg-slate-200 px-4 py-2 dark:bg-muted/30", headerClassName)}>
        <span className="text-[11px] leading-4 font-mono uppercase tracking-[0.14em] text-muted-foreground">
          {title}
        </span>
        {badge && badgeTooltip && (
          <Popover>
            <PopoverTrigger asChild>
              <button
                type="button"
                className="cursor-pointer rounded-[2px] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1"
                aria-label={badgeTooltip}
              >
                {badge}
              </button>
            </PopoverTrigger>
            <PopoverContent>{badgeTooltip}</PopoverContent>
          </Popover>
        )}
        {trailing && <span className="ml-auto">{trailing}</span>}
      </header>
      <div className={cn("px-4 py-3", bodyClassName)}>
        {children}
      </div>
    </section>
  );
}
