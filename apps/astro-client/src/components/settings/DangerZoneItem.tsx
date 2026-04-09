import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

interface DangerZoneItemProps {
  title: string;
  description: string;
  actionLabel: string;
  onAction: () => void;
  disabled?: boolean;
  disabledReason?: string;
  children?: ReactNode;
}

export function DangerZoneItem({
  title,
  description,
  actionLabel,
  onAction,
  disabled,
  disabledReason,
  children,
}: DangerZoneItemProps) {
  const button = (
    <Button
      variant="outline"
      className="shrink-0 border-destructive/30 bg-surface text-destructive hover:bg-destructive/[0.08] hover:text-destructive active:bg-destructive/15 active:text-destructive"
      onClick={onAction}
      disabled={disabled}
    >
      {actionLabel}
    </Button>
  );

  return (
    <div className="flex items-center justify-between gap-4 rounded-lg border border-destructive/30 bg-destructive/5 px-5 py-4">
      <div>
        <div className="text-[13px] font-semibold text-foreground">{title}</div>
        <p className="text-[12px] text-muted-foreground">{description}</p>
      </div>
      {disabled && disabledReason ? (
        <TooltipProvider delayDuration={300}>
          <Tooltip>
            <TooltipTrigger asChild>
              <span>{button}</span>
            </TooltipTrigger>
            <TooltipContent>{disabledReason}</TooltipContent>
          </Tooltip>
        </TooltipProvider>
      ) : (
        button
      )}
      {children}
    </div>
  );
}
