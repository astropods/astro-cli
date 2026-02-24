import type { ReactNode } from "react";

export interface SidebarSectionProps {
  title: string;
  children: ReactNode;
}

export function SidebarSection({ title, children }: SidebarSectionProps) {
  return (
    <div>
      <span className="text-xs text-muted-foreground mb-3 block">{title}</span>
      {children}
    </div>
  );
}
