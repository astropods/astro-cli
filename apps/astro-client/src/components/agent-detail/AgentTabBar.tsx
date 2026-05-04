import { NavLink, useLocation, useNavigate, useParams } from "react-router";
import { motion } from "motion/react";
import { Activity, ChevronDown, Layers, Settings, type LucideIcon } from "lucide-react";

const TABS: { label: string; path: string; icon: LucideIcon }[] = [
  { label: "Monitor", path: "monitor", icon: Activity },
  { label: "Deployments", path: "deployments", icon: Layers },
  { label: "Configure", path: "configure", icon: Settings },
];

export function AgentTabBar() {
  const { account, deploymentId } = useParams<{ account: string; deploymentId: string }>();
  const basePath = `/${account}/agents/${deploymentId}`;
  const location = useLocation();
  const navigate = useNavigate();
  const activeTab = TABS.find((t) => location.pathname.endsWith(`/${t.path}`));

  const ActiveIcon = activeTab?.icon ?? Activity;

  return (
    <nav className="@container pointer-events-none absolute inset-x-0 top-4 z-20">
      {/* Full tab bar — hidden below 600px */}
      <div className="flex justify-center @max-[1000px]:hidden">
        <div className="pointer-events-auto flex items-center gap-1 rounded-[8px] dark:rounded-md bg-background p-1 dark:border-[1.5px] dark:border-border">
          {TABS.map((tab) => {
            const isActive = tab.path === activeTab?.path;
            return (
              <NavLink
                key={tab.path}
                to={`${basePath}/${tab.path}`}
                className="relative rounded-sm px-4 py-1.5 text-body-sm font-medium transition-colors"
              >
                {isActive && (
                  <motion.span
                    layoutId="agent-tab-highlight"
                    className="absolute inset-0 rounded-sm bg-primary"
                    transition={{ type: "spring", bounce: 0.15, duration: 0.4 }}
                  />
                )}
                <span className={`relative z-10 flex items-center gap-2 text-sm tracking-wide ${isActive ? "text-primary-foreground" : "text-foreground"}`}>
                  <tab.icon className="size-4" />
                  {tab.label}
                </span>
              </NavLink>
            );
          })}
        </div>
      </div>

      {/* Compact dropdown — shown below 600px */}
      <div className="hidden justify-center @max-[1000px]:flex @max-[700px]:justify-end @max-[700px]:pr-3">
        <div className="pointer-events-auto relative rounded-[8px] dark:rounded-md bg-background dark:border-[1.5px] dark:border-border">
          <select
            value={activeTab?.path ?? ""}
            onChange={(e) => void navigate(`${basePath}/${e.target.value}`)}
            className="appearance-none bg-transparent py-1.5 pl-9 pr-8 text-sm font-medium tracking-wide text-foreground outline-none"
          >
            {TABS.map((tab) => (
              <option key={tab.path} value={tab.path}>
                {tab.label}
              </option>
            ))}
          </select>
          <ActiveIcon className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-foreground" />
          <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 size-3.5 -translate-y-1/2 text-foreground" />
        </div>
      </div>
    </nav>
  );
}
