import type { ReactNode } from "react";
import { Link } from "react-router";
import { ChevronRight } from "lucide-react";

export interface BreadcrumbItem {
  label: ReactNode;
  to?: string;
}

export interface PageBreadcrumbProps {
  items: BreadcrumbItem[];
  actions?: ReactNode;
}

export function PageBreadcrumb({ items, actions }: PageBreadcrumbProps) {
  return (
    <div className="sticky top-0 z-10 flex items-center justify-between px-6 h-[52px] bg-stone-200 border-b border-stone-300 dark:bg-background dark:border-border">
      <div className="flex items-center gap-2 font-mono text-mono-sm text-muted-foreground">
        {items.map((item, i) => (
          <span key={i} className="flex items-center gap-2">
            {i > 0 && <ChevronRight className="size-3 text-[var(--faint-foreground)]" />}
            {item.to ? (
              <Link to={item.to} className="hover:text-foreground transition-colors">
                {item.label}
              </Link>
            ) : (
              <span className="text-foreground font-semibold">{item.label}</span>
            )}
          </span>
        ))}
      </div>
      {actions && <div className="flex items-center gap-1">{actions}</div>}
    </div>
  );
}
