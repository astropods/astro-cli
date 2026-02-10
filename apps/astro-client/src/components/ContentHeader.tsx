import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

export interface BreadcrumbItem {
  label: string;
  to?: string;
}

export interface ContentHeaderProps {
  children?: ReactNode;
  className?: string;
}

export function ContentHeader({ children, className }: ContentHeaderProps) {
  return (
    <div
      className={cn(
        "flex items-center gap-2 border-b border-border px-5 py-4",
        className,
      )}
    >
      {children}
    </div>
  );
}
