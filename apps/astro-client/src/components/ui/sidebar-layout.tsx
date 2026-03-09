import { NavLink } from "react-router";
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
  return (
    <nav className={cn("flex w-full gap-1 overflow-x-auto md:w-36 md:shrink-0 md:flex-col md:overflow-x-visible", className)}>
      {label && (
        <span className="hidden md:block text-mono-sm font-mono uppercase tracking-widest text-ink-faint px-3 pb-1">
          {label}
        </span>
      )}
      {children}
    </nav>
  );
}

/* ── Individual nav item ── */

const navItemBase =
  "whitespace-nowrap rounded-sm px-3 py-1.5 text-left text-[13px] transition-colors cursor-pointer";
const navItemActive = "bg-stone-300 text-ink font-medium";
const navItemInactive = "text-ink-muted font-normal hover:bg-muted/50 hover:text-foreground";

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
    <div className={cn("flex flex-1 flex-col gap-6", className)}>
      {children}
    </div>
  );
}
