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

export function SidebarNavItem({
  active,
  className,
  children,
  ...props
}: {
  active?: boolean;
  className?: string;
  children: React.ReactNode;
} & React.ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      type="button"
      className={cn(
        "whitespace-nowrap rounded-sm px-3 py-1.5 text-left text-[13px] transition-colors cursor-pointer",
        active
          ? "bg-stone-300 text-ink font-medium"
          : "text-ink-muted font-normal hover:bg-muted/50 hover:text-foreground",
        className,
      )}
      {...props}
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
