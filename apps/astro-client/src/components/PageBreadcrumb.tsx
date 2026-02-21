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
    <div className="sticky top-0 z-10 flex items-center justify-between px-6 py-3 min-h-[52px] bg-white border-b border-border">
      <div className="flex items-center gap-2 text-sm text-stone-500">
        {items.map((item, i) => (
          <span key={i} className="flex items-center gap-2">
            {i > 0 && <ChevronRight className="h-3.5 w-3.5 text-stone-400" />}
            {item.to ? (
              <Link to={item.to} className="hover:text-foreground transition-colors">
                {item.label}
              </Link>
            ) : (
              <span className="text-foreground font-medium">{item.label}</span>
            )}
          </span>
        ))}
      </div>
      {actions && <div className="flex items-center gap-1">{actions}</div>}
    </div>
  );
}
