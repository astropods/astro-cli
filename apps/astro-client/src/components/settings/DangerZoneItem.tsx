import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";

interface DangerZoneItemProps {
  title: string;
  description: string;
  actionLabel: string;
  onAction: () => void;
  children?: ReactNode;
}

export function DangerZoneItem({
  title,
  description,
  actionLabel,
  onAction,
  children,
}: DangerZoneItemProps) {
  return (
    <div className="flex items-center justify-between gap-4 rounded-lg border border-destructive/30 bg-destructive/5 px-5 py-4">
      <div>
        <div className="text-[13px] font-semibold text-foreground">{title}</div>
        <p className="text-[12px] text-muted-foreground">{description}</p>
      </div>
      <Button
        variant="outline"
        className="shrink-0 border-destructive/30 bg-surface text-destructive hover:bg-destructive/[0.08] hover:text-destructive active:bg-destructive/15 active:text-destructive"
        onClick={onAction}
      >
        {actionLabel}
      </Button>
      {children}
    </div>
  );
}
