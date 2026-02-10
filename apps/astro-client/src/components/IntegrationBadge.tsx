import type { CSSProperties, ReactNode } from "react";
import { cn } from "@/lib/utils";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

export interface IntegrationBadgeProps {
  name: string;
  icon: ReactNode;
  className?: string;
  style?: CSSProperties;
}

export function IntegrationBadge({
  name,
  icon,
  className,
  style,
}: IntegrationBadgeProps) {
  return (
    <TooltipProvider delayDuration={300}>
      <Tooltip>
        <TooltipTrigger asChild>
          <div
            style={style}
            className={cn(
              "relative inline-flex items-center justify-center rounded-[6px] border-2 border-white dark:border-black bg-muted p-1",
              className,
            )}
          >
            <div className="size-5">{icon}</div>
          </div>
        </TooltipTrigger>
        <TooltipContent side="top">{name}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
