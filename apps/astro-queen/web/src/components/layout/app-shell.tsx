import { useState } from "react";
import { NavLink, Outlet } from "react-router";
import { cn } from "@/lib/utils";
import {
  LayoutDashboard,
  Activity,
  Users,
  Bot,

  Wifi,
  Send,
  Gauge,
  Star,
  UserCircle,
  ClipboardList,
  Zap,
  Waves,
  ArrowUpCircle,
  MessageSquare,
  ExternalLink,
  ChevronRight,
  PanelLeftClose,
  PanelLeftOpen,
} from "lucide-react";
import { useEnv } from "@/api/admin";

const sections = [
  {
    key: "admin",
    label: "Admin",
    links: [
      { to: "/admin/quota-requests", label: "Quota Requests", icon: ArrowUpCircle },
      { to: "/admin/deployments", label: "Deployments", icon: LayoutDashboard },
      { to: "/admin/accounts", label: "Accounts", icon: Users },
      { to: "/admin/blueprints", label: "Blueprints", icon: Bot },

      { to: "/admin/devices", label: "Devices", icon: Wifi },
      { to: "/admin/api-client", label: "API Client", icon: Send },
      { to: "/admin/feedback", label: "Feedback", icon: MessageSquare },
      { to: "/admin/river-ui", label: "River UI", icon: Waves },
    ],
  },
  {
    key: "openmeter",
    label: "OpenMeter",
    homeTo: "/openmeter",
    links: [
      { to: "/openmeter/dashboard", label: "Dashboard", icon: Activity },
      { to: "/openmeter/meters", label: "Meters", icon: Gauge },
      { to: "/openmeter/features", label: "Features", icon: Star },
      { to: "/openmeter/customers", label: "Customers", icon: UserCircle },
      { to: "/openmeter/plans", label: "Plans", icon: ClipboardList },
      { to: "/openmeter/events", label: "Events", icon: Zap },
    ],
  },
];

function TreeSection({
  section,
  open,
  onToggle,
  collapsed,
}: {
  section: (typeof sections)[number] & { homeTo?: string };
  open: boolean;
  onToggle: () => void;
  collapsed: boolean;
}) {
  if (collapsed) {
    return (
      <div className="flex flex-col items-center gap-1">
        {section.links.map((link) =>
          "external" in link && link.external ? (
            <a
              key={link.to}
              href={link.to}
              target="_blank"
              rel="noopener noreferrer"
              title={link.label}
              className="flex size-7 items-center justify-center rounded text-muted-foreground hover:bg-glass-light hover:text-foreground transition-colors"
            >
              <link.icon className="size-3.5" />
            </a>
          ) : (
            <NavLink
              key={link.to}
              to={link.to}
              title={link.label}
              className={({ isActive }) =>
                cn(
                  "flex size-7 items-center justify-center rounded transition-colors",
                  isActive
                    ? "bg-pollen/80 text-honey-dark"
                    : "text-muted-foreground hover:bg-glass-light hover:text-foreground"
                )
              }
            >
              <link.icon className="size-3.5" />
            </NavLink>
          )
        )}
      </div>
    );
  }

  return (
    <div>
      <div className="flex items-center gap-0.5">
        <button
          onClick={onToggle}
          className="shrink-0 text-muted-foreground hover:text-foreground transition-colors"
        >
          <ChevronRight
            className={cn("size-3 transition-transform", open && "rotate-90")}
          />
        </button>
        {section.homeTo ? (
          <NavLink
            to={section.homeTo}
            className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground hover:text-foreground transition-colors"
          >
            {section.label}
          </NavLink>
        ) : (
          <button
            onClick={onToggle}
            className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground hover:text-foreground transition-colors"
          >
            {section.label}
          </button>
        )}
      </div>
      {open && (
        <nav className="ml-1.5 border-l border-glass-border-honey pl-2 mt-0.5">
          {section.links.map((link) =>
            "external" in link && link.external ? (
              <a
                key={link.to}
                href={link.to}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-1.5 rounded px-1.5 py-[3px] text-xs text-muted-foreground hover:bg-glass-light hover:text-foreground transition-colors"
              >
                <link.icon className="size-3 shrink-0" />
                {link.label}
                <ExternalLink className="ml-auto size-2.5 opacity-50" />
              </a>
            ) : (
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
            )
          )}
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
  const [collapsed, setCollapsed] = useState(false);
  const { data: envData } = useEnv();

  const toggle = (key: string) =>
    setOpenSections((s) => ({ ...s, [key]: !s[key] }));

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
        <div className={cn("flex flex-1 flex-col gap-3 overflow-y-auto", !collapsed && "px-1")}>
          {sections.map((s) => (
            <TreeSection
              key={s.key}
              section={s}
              open={openSections[s.key] ?? true}
              onToggle={() => toggle(s.key)}
              collapsed={collapsed}
            />
          ))}
        </div>
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
