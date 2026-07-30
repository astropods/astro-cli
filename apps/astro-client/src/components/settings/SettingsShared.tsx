import React from "react";
import { CheckIcon } from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";

const headingClass = {
  h1: "text-heading-1",
  h2: "text-heading-2",
  h3: "text-heading-3",
} as const;

export function SectionHeader({
  as: Heading = "h2",
  title,
  subtitle,
  action,
  className,
}: {
  as?: "h1" | "h2" | "h3";
  title: React.ReactNode;
  subtitle: string;
  action?: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("pb-4 mb-4 border-b border-border", className)}>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
        <div className="space-y-1 min-w-0">
          <Heading className={`${headingClass[Heading]} text-foreground`}>{title}</Heading>
          <p className="text-[13px] text-pretty text-muted-foreground">{subtitle}</p>
        </div>
        {action && <div className="shrink-0">{action}</div>}
      </div>
    </div>
  );
}

export function SavedIndicator({ visible }: { visible: boolean }) {
  if (!visible) return null;
  return (
    <span className="flex items-center gap-1 text-[13px] text-muted-foreground">
      <CheckIcon className="size-3.5" />
      Saved
    </span>
  );
}
