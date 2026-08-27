import * as React from "react";
import { NavLink, useLocation } from "react-router";
import { ChevronDownIcon } from "lucide-react";
import { cn } from "@/lib/utils";

/* ── Layout shell: sidebar + body ── */

export function SidebarLayout({
  className,
  children,
}: {
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div className={cn("flex flex-col gap-6 md:flex-row md:gap-6", className)}>
      {children}
    </div>
  );
}

/* ── Sidebar nav container ── */

export function SidebarNav({
  label,
  className,
  children,
}: {
  label?: string;
  className?: string;
  children: React.ReactNode;
}) {
  const pillsRef = React.useRef<HTMLDivElement>(null);
  const [overflowing, setOverflowing] = React.useState(false);

  // Check overflow synchronously after every render + on resize
  const checkOverflow = React.useCallback(() => {
    const el = pillsRef.current;
    if (!el) return;
    setOverflowing(el.scrollWidth > el.clientWidth);
  }, []);

  React.useLayoutEffect(checkOverflow);

  React.useEffect(() => {
    const el = pillsRef.current;
    if (!el) return;
    const ro = new ResizeObserver(checkOverflow);
    ro.observe(el);
    return () => ro.disconnect();
  }, [checkOverflow]);

  return (
    <nav
      className={cn(
        "flex w-full flex-col gap-1 md:w-36 md:shrink-0 md:pt-2 md:overflow-x-visible",
        className,
      )}
    >
      {label && (
        <span className="hidden md:block text-mono-sm font-mono uppercase tracking-widest text-faint-foreground px-3 pb-1">
          {label}
        </span>
      )}

      {/* Mobile: pills (also serves as measurement container when hidden) */}
      <div
        ref={pillsRef}
        aria-hidden={overflowing || undefined}
        className={cn(
          "flex gap-1 md:hidden",
          overflowing
            ? "h-0 overflow-hidden pointer-events-none"
            : "overflow-x-auto",
        )}
      >
        {children}
      </div>

      {/* Mobile: dropdown when pills overflow */}
      {overflowing && (
        <MobileNavDropdown label={label}>{children}</MobileNavDropdown>
      )}

      {/* Desktop: vertical sidebar */}
      <div className="hidden md:flex md:flex-col md:gap-1">{children}</div>
    </nav>
  );
}

/* ── Mobile nav dropdown ── */

function MobileNavDropdown({
  label,
  children,
}: {
  label?: string;
  children: React.ReactNode;
}) {
  const [open, setOpen] = React.useState(false);
  const ref = React.useRef<HTMLDivElement>(null);
  const location = useLocation();

  // Close when route changes
  React.useEffect(() => {
    setOpen(false);
  }, [location.pathname]);

  // Close on click outside
  React.useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open]);

  const activeLabel = getActiveLabel(children, location.pathname);

  return (
    <div ref={ref} className="relative md:hidden">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className={cn(
          navItemBase,
          "flex w-full items-center justify-between gap-2 bg-secondary text-foreground",
        )}
      >
        <span className="truncate">{activeLabel ?? label ?? "Menu"}</span>
        <ChevronDownIcon
          className={cn(
            "size-3.5 shrink-0 transition-transform",
            open && "rotate-180",
          )}
        />
      </button>
      {open && (
        <div
          className="absolute top-full left-0 right-0 z-10 mt-1 flex flex-col gap-0.5 rounded-md border bg-popover p-1 shadow-md"
          onClick={() => setOpen(false)}
        >
          {children}
        </div>
      )}
    </div>
  );
}

/** Extract the active item's children for the dropdown trigger label. */
function getActiveLabel(
  children: React.ReactNode,
  pathname: string,
): React.ReactNode | null {
  let active: React.ReactNode = null;
  function walk(nodes: React.ReactNode) {
    React.Children.forEach(nodes, (child) => {
      if (!React.isValidElement(child)) return;
      const props = child.props as Record<string, unknown>;
      // NavLink item
      if (typeof props.to === "string") {
        if (pathname.startsWith(props.to)) active = props.children as React.ReactNode;
        return;
      }
      // Button item
      if (props.active) {
        active = props.children as React.ReactNode;
        return;
      }
      // Group, fragment, or any other wrapper
      walk(props.children as React.ReactNode);
    });
  }
  walk(children);
  return active;
}

/* ── Nav section: a labelled group of items ── */

export function SidebarNavGroup({
  label,
  className,
  children,
}: {
  label: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div className={cn("contents md:flex md:flex-col md:gap-0.5 md:pt-4 md:first:pt-0", className)}>
      <span className="hidden md:block text-body-sm text-faint-foreground px-3 pb-1">
        {label}
      </span>
      {children}
    </div>
  );
}

export function SidebarNavDivider() {
  return <div aria-hidden="true" className="hidden md:block md:my-3 border-t border-border" />;
}

/** A nav row for a section that isn't built yet: shown, never navigable. */
export function SidebarNavPlaceholder({
  note,
  children,
}: {
  note: string;
  children: React.ReactNode;
}) {
  return (
    <span
      aria-disabled="true"
      className={cn(navItemBase, "flex cursor-default items-center gap-2 text-faint-foreground")}
    >
      {children}
      <span className="font-mono text-mono-xs text-faint-foreground">{note}</span>
    </span>
  );
}

/* ── Individual nav item ── */

const navItemBase =
  "whitespace-nowrap rounded-sm px-3 py-1.5 text-left text-[13px] transition-colors cursor-pointer";
const navItemActive = "bg-secondary text-foreground font-medium";
const navItemInactive = "text-foreground font-normal hover:bg-muted/50";

type SidebarNavLinkProps = {
  to: string;
  active?: never;
  className?: string;
  children: React.ReactNode;
};

type SidebarNavButtonProps = {
  to?: never;
  active?: boolean;
  className?: string;
  children: React.ReactNode;
} & React.ButtonHTMLAttributes<HTMLButtonElement>;

export function SidebarNavItem(props: SidebarNavLinkProps | SidebarNavButtonProps) {
  const { to, className, children } = props;

  if (to) {
    return (
      <NavLink
        to={to}
        className={({ isActive }) =>
          cn(navItemBase, isActive ? navItemActive : navItemInactive, className)
        }
      >
        {children}
      </NavLink>
    );
  }

  const { active, ...buttonProps } = props as SidebarNavButtonProps;
  return (
    <button
      type="button"
      aria-current={active ? "true" : undefined}
      className={cn(navItemBase, active ? navItemActive : navItemInactive, className)}
      {...buttonProps}
    >
      {children}
    </button>
  );
}

/* ── Body area (right side) ── */

export function SidebarBody({
  className,
  children,
}: {
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div className={cn("flex min-w-0 flex-1 flex-col gap-6 md:pt-2", className)}>
      {children}
    </div>
  );
}
