import type { ReactNode } from "react";

export function Chip({ children }: { children: ReactNode }) {
  return (
    <div className="inline-flex items-center gap-1.5 h-7 rounded-sm border border-border bg-white px-2 text-body-sm text-muted-foreground">
      {children}
    </div>
  );
}
