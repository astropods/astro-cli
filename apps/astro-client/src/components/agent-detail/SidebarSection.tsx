import type { ReactNode } from "react";

export interface SidebarSectionProps {
  title: string;
  children: ReactNode;
  className?: string;
  bodyClassName?: string;
}

export function SidebarSection({ title, children, className, bodyClassName }: SidebarSectionProps) {
  return (
    <section className={`overflow-hidden rounded-md border border-border-strong bg-surface ${className ?? ""}`}>
      <header className="border-b border-border-strong bg-stone-200 px-4 py-2.5 dark:bg-muted/30">
        <span className="block text-[11px] leading-4 font-mono uppercase tracking-[0.14em] text-muted-foreground">
          {title}
        </span>
      </header>
      <div className={`px-4 py-3.5 ${bodyClassName ?? ""}`}>
        {children}
      </div>
    </section>
  );
}
