import type { ReactNode } from "react";
import { Link } from "react-router";
import { ChevronRight } from "lucide-react";

export interface BreadcrumbItem {
  label: ReactNode;
  to?: string;
}

export interface PageBreadcrumbProps {
  items: BreadcrumbItem[];
  mobileItems?: BreadcrumbItem[];
  actions?: ReactNode;
  mobileActions?: ReactNode;
}

function BreadcrumbTrail({ items }: { items: BreadcrumbItem[] }) {
  return (
    <>
      {items.map((item, i) => (
        <span key={i} className="flex min-w-0 items-center gap-2">
          {i > 0 && (
            <ChevronRight className="size-3 shrink-0 text-[var(--faint-foreground)]" />
          )}
          {item.to ? (
            <Link
              to={item.to}
              className="break-all hover:text-foreground transition-colors"
            >
              {item.label}
            </Link>
          ) : (
            <span className="break-all font-medium text-foreground">
              {item.label}
            </span>
          )}
        </span>
      ))}
    </>
  );
}

export function PageBreadcrumb({ items, mobileItems, actions, mobileActions }: PageBreadcrumbProps) {
  const resolvedMobileItems = mobileItems ?? items;

  return (
    <div className="sticky top-0 z-10 flex min-h-[52px] flex-wrap items-center justify-between gap-x-3 gap-y-2 border-b border-stone-300 bg-surface px-6 py-2 dark:border-border">
      <div className="hidden sm:flex min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1 font-mono text-mono-sm text-muted-foreground">
        <BreadcrumbTrail items={items} />
      </div>
      <div className="flex sm:hidden min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1 font-mono text-mono-sm text-muted-foreground">
        <BreadcrumbTrail items={resolvedMobileItems} />
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
