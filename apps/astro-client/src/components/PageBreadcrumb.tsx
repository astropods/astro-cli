import type { ReactNode } from "react";
import { Link } from "react-router";
import { ChevronRight, Ellipsis } from "lucide-react";

export interface BreadcrumbItem {
  label: ReactNode;
  to?: string;
}

export interface PageBreadcrumbProps {
  items: BreadcrumbItem[];
  actions?: ReactNode;
  mobileActions?: ReactNode;
}

export function PageBreadcrumb({ items, actions, mobileActions }: PageBreadcrumbProps) {
  return (
    <div className="sticky top-0 z-10 flex h-[52px] items-center justify-between border-b border-stone-300 bg-surface px-6 dark:border-border">
      <div className="flex items-center gap-2 font-mono text-mono-sm text-muted-foreground">
        {items.map((item, i) => (
          <span key={i} className="flex items-center gap-2">
            {i > 0 && <ChevronRight className="size-3 text-[var(--faint-foreground)]" />}
            {item.to ? (
              <Link to={item.to} className="hover:text-foreground transition-colors">
                <span className="hidden sm:inline">{item.label}</span>
                <Ellipsis className="size-4 sm:hidden" />
              </Link>
            ) : (
              <span className="font-medium text-foreground">{item.label}</span>
            )}
          </span>
        ))}
      </div>
      {actions && (
        <>
          <div className="hidden sm:flex items-center gap-2">{actions}</div>
          {mobileActions && <div className="flex sm:hidden items-center gap-2">{mobileActions}</div>}
        </>
      )}
    </div>
  );
}
