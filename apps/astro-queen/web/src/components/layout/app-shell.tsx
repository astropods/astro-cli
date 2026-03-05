import { useState } from "react";
import { NavLink, Outlet } from "react-router";
import { cn } from "@/lib/utils";
import {
  LayoutDashboard,
  Users,
  Bot,
  Server,
  Gauge,
  Star,
  UserCircle,
  Zap,
  ChevronRight,
} from "lucide-react";

const sections = [
  {
    key: "admin",
    label: "Admin",
    links: [
      { to: "/admin/deployments", label: "Deployments", icon: LayoutDashboard },
      { to: "/admin/accounts", label: "Accounts", icon: Users },
      { to: "/admin/agents", label: "Agents", icon: Bot },
      { to: "/admin/cluster", label: "Cluster", icon: Server },
    ],
  },
  {
    key: "openmeter",
    label: "OpenMeter",
    links: [
      { to: "/openmeter/meters", label: "Meters", icon: Gauge },
      { to: "/openmeter/features", label: "Features", icon: Star },
      { to: "/openmeter/customers", label: "Customers", icon: UserCircle },
      { to: "/openmeter/events", label: "Events", icon: Zap },
    ],
  },
];

function TreeSection({
  section,
  open,
  onToggle,
}: {
  section: (typeof sections)[number];
  open: boolean;
  onToggle: () => void;
}) {
  return (
    <div>
      <button
        onClick={onToggle}
        className="flex w-full items-center gap-1 py-0.5 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground hover:text-foreground transition-colors"
      >
        <ChevronRight
          className={cn("size-3 shrink-0 transition-transform", open && "rotate-90")}
        />
        {section.label}
      </button>
      {open && (
        <nav className="ml-1.5 border-l border-glass-border-honey pl-2 mt-0.5">
          {section.links.map((link) => (
            <NavLink
              key={link.to}
              to={link.to}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-1.5 rounded px-1.5 py-[3px] text-xs transition-colors",
                  isActive
                    ? "bg-pollen/80 text-honey-dark"
                    : "text-muted-foreground hover:bg-glass-light hover:text-foreground"
                )
              }
            >
              <link.icon className="size-3 shrink-0" />
              {link.label}
            </NavLink>
          ))}
        </nav>
      )}
    </div>
  );
}

export function AppShell() {
  const [openSections, setOpenSections] = useState<Record<string, boolean>>({
    admin: true,
    openmeter: true,
  });

  const toggle = (key: string) =>
    setOpenSections((s) => ({ ...s, [key]: !s[key] }));

  return (
    <div className="flex h-screen">
      <aside className="glass-heavy flex w-44 shrink-0 flex-col px-2 py-3">
        <div className="mb-4 px-1">
          <h1 className="text-sm font-bold text-honey-dark">Queen 🐝</h1>
        </div>
        <div className="flex flex-1 flex-col gap-3 overflow-y-auto px-1">
          {sections.map((s) => (
            <TreeSection
              key={s.key}
              section={s}
              open={openSections[s.key] ?? true}
              onToggle={() => toggle(s.key)}
            />
          ))}
        </div>
        <div className="mt-2 border-t border-glass-border-honey px-1 pt-2">
          <p className="text-[9px] text-muted-foreground">astro admin toolkit</p>
        </div>
      </aside>
      <main className="flex-1 overflow-y-auto p-6">
        <Outlet />
      </main>
    </div>
  );
}
