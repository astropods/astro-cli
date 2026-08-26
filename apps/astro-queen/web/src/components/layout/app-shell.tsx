import { useState } from "react";
import { NavLink, Outlet } from "react-router";
import { cn } from "@/lib/utils";
import {
  LayoutDashboard,
  Users,
  Bot,
  Send,
  Waves,
  ArrowUpCircle,
  ArrowLeftRight,
  Globe,
  MessageSquare,
  Bell,
  Boxes,
  ShieldAlert,
  ExternalLink,
  PanelLeftClose,
  PanelLeftOpen,
} from "lucide-react";
import { useEnv } from "@/api/admin";

type NavItem = {
  to: string;
  label: string;
  icon: typeof LayoutDashboard;
  external?: boolean;
};

const links: NavItem[] = [
  { to: "/admin/accounts", label: "Accounts", icon: Users },
  { to: "/admin/quota-requests", label: "Quota Requests", icon: ArrowUpCircle },
  { to: "/admin/deployments", label: "Deployments", icon: LayoutDashboard },
  { to: "/admin/resources", label: "Resources", icon: Boxes },
  { to: "/admin/alerts", label: "Alerts", icon: Bell },
  { to: "/admin/audit", label: "System Audit", icon: ShieldAlert },
  { to: "/admin/clusters", label: "Clusters", icon: Globe },
  { to: "/admin/migrations", label: "Migrations", icon: ArrowLeftRight },
  { to: "/admin/blueprints", label: "Blueprints", icon: Bot },
  { to: "/admin/feedback", label: "Feedback", icon: MessageSquare },
  { to: "/admin/jobs", label: "Jobs", icon: Waves },
  { to: "/admin/api-client", label: "API Client", icon: Send },
];

function NavRow({ link, collapsed }: { link: NavItem; collapsed: boolean }) {
  if (link.external) {
    return (
      <a
        href={link.to}
        target="_blank"
        rel="noopener noreferrer"
        title={collapsed ? link.label : undefined}
        className={cn(
          "flex items-center rounded text-muted-foreground hover:bg-glass-light hover:text-foreground transition-colors",
          collapsed
            ? "size-7 justify-center"
            : "gap-1.5 px-1.5 py-[3px] text-xs"
        )}
      >
        <link.icon className={collapsed ? "size-3.5" : "size-3 shrink-0"} />
        {!collapsed && (
          <>
            {link.label}
            <ExternalLink className="ml-auto size-2.5 opacity-50" />
          </>
        )}
      </a>
    );
  }

  return (
    <NavLink
      to={link.to}
      title={collapsed ? link.label : undefined}
      className={({ isActive }) =>
        cn(
          "flex items-center rounded transition-colors",
          collapsed ? "size-7 justify-center" : "gap-1.5 px-1.5 py-[3px] text-xs",
          isActive
            ? "bg-pollen/80 text-honey-dark"
            : "text-muted-foreground hover:bg-glass-light hover:text-foreground"
        )
      }
    >
      <link.icon className={collapsed ? "size-3.5" : "size-3 shrink-0"} />
      {!collapsed && link.label}
    </NavLink>
  );
}

export function AppShell() {
  const [collapsed, setCollapsed] = useState(false);
  const { data: envData } = useEnv();

  return (
    <div className="flex h-screen">
      <aside
        className={cn(
          "glass-heavy flex shrink-0 flex-col py-3 transition-all duration-200",
          collapsed ? "w-11 px-1" : "w-44 px-2"
        )}
      >
        <div className={cn("mb-4 flex items-center", collapsed ? "justify-center" : "justify-between px-1")}>
          {!collapsed && (
            <div>
              <h1 className="text-sm font-bold text-honey-dark">Queen 🐝</h1>
              {envData?.env && (
                <p className="text-[9px] text-muted-foreground">{envData.env}</p>
              )}
            </div>
          )}
          <button
            onClick={() => setCollapsed((c) => !c)}
            className="text-muted-foreground hover:text-foreground transition-colors"
            title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          >
            {collapsed ? <PanelLeftOpen className="size-3.5" /> : <PanelLeftClose className="size-3.5" />}
          </button>
        </div>
        <nav
          className={cn(
            "flex flex-1 flex-col overflow-y-auto",
            collapsed ? "items-center gap-1" : "gap-0.5 px-1"
          )}
        >
          {links.map((link) => (
            <NavRow key={link.to} link={link} collapsed={collapsed} />
          ))}
        </nav>
        {!collapsed && (
          <div className="mt-2 border-t border-glass-border-honey px-1 pt-2">
            <p className="text-[9px] text-muted-foreground">astro admin toolkit</p>
          </div>
        )}
      </aside>
      <main className="flex-1 overflow-y-auto p-6">
        <Outlet />
      </main>
    </div>
  );
}
