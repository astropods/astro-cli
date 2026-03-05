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
} from "lucide-react";

const adminLinks = [
  { to: "/admin/deployments", label: "Deployments", icon: LayoutDashboard },
  { to: "/admin/accounts", label: "Accounts", icon: Users },
  { to: "/admin/agents", label: "Agents", icon: Bot },
  { to: "/admin/cluster", label: "Cluster", icon: Server },
];

const openmeterLinks = [
  { to: "/openmeter/meters", label: "Meters", icon: Gauge },
  { to: "/openmeter/features", label: "Features", icon: Star },
  { to: "/openmeter/customers", label: "Customers", icon: UserCircle },
  { to: "/openmeter/events", label: "Events", icon: Zap },
];

function SidebarSection({
  title,
  links,
}: {
  title: string;
  links: { to: string; label: string; icon: React.ComponentType<{ className?: string }> }[];
}) {
  return (
    <div>
      <h3 className="mb-2 px-3 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
        {title}
      </h3>
      <nav className="space-y-0.5">
        {links.map((link) => (
          <NavLink
            key={link.to}
            to={link.to}
            className={({ isActive }) =>
              cn(
                "flex items-center gap-2.5 rounded-md px-3 py-1.5 text-sm transition-colors",
                isActive
                  ? "bg-pollen text-honey-dark border border-honey/20"
                  : "text-muted-foreground hover:bg-glass-light hover:text-foreground border border-transparent"
              )
            }
          >
            <link.icon className="size-4 shrink-0" />
            {link.label}
          </NavLink>
        ))}
      </nav>
    </div>
  );
}

export function AppShell() {
  return (
    <div className="flex h-screen">
      <aside className="glass-heavy flex w-56 shrink-0 flex-col px-3 py-5">
        <div className="mb-8 px-3">
          <h1 className="text-lg font-bold text-honey-dark">Queen 🐝</h1>
          <p className="text-[11px] text-muted-foreground">Running the hive</p>
        </div>
        <div className="flex flex-1 flex-col gap-6 overflow-y-auto">
          <SidebarSection title="Admin" links={adminLinks} />
          <SidebarSection title="OpenMeter" links={openmeterLinks} />
        </div>
        <div className="mt-4 border-t border-glass-border-honey px-3 pt-3">
          <p className="text-[10px] text-muted-foreground">astro admin toolkit</p>
        </div>
      </aside>
      <main className="flex-1 overflow-y-auto p-6">
        <Outlet />
      </main>
    </div>
  );
}
