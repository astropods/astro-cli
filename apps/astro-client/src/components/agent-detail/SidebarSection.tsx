import type { ReactNode } from "react";

export interface SidebarSectionProps {
  title: string;
  children: ReactNode;
}

export function SidebarSection({ title, children }: SidebarSectionProps) {
  return (
    <div>
      <span className="text-[11px] text-[var(--faint-foreground)] mb-2 block">{title}</span>
      {children}
    </div>
  );
}
