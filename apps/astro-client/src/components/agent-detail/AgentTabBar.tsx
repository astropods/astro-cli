import { NavLink, useLocation, useNavigate, useParams } from "react-router";
import { motion } from "motion/react";
import { Activity, FlaskConical, Layers, ScrollText, Settings, type LucideIcon } from "lucide-react";
import { DeploymentTab } from "@/lib/routes";
import { useExperiments, type Experiments } from "@/lib/experiments";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

const BASE_TABS: { label: string; path: DeploymentTab; icon: LucideIcon; experiment?: keyof Experiments }[] = [
  { label: "Monitor", path: DeploymentTab.Monitor, icon: Activity },
  { label: "Traces", path: DeploymentTab.Traces, icon: ScrollText },
  { label: "Deployments", path: DeploymentTab.Deployment, icon: Layers },
  { label: "Configure", path: DeploymentTab.Configure, icon: Settings },
  { label: "Eval", path: DeploymentTab.Dataset, icon: FlaskConical, experiment: "evals" },
];

export function AgentTabBar() {
  const { account, deploymentId } = useParams<{ account: string; deploymentId: string }>();
  const basePath = `/${account}/agents/${deploymentId}`;
  const location = useLocation();
  const navigate = useNavigate();
  const { experiments } = useExperiments();
  const TABS = BASE_TABS.filter((t) => !t.experiment || experiments[t.experiment]);
  const activeTab = TABS.find((t) => location.pathname.endsWith(`/${t.path}`));

  return (
    <nav className="pointer-events-none min-w-0">
      {/* Full tab bar — hidden once the centered controls would crowd. */}
      <div className="flex max-w-full justify-center max-[1280px]:hidden">
        <div className="pointer-events-auto flex max-w-full items-center gap-1 rounded-[8px] bg-background p-1 dark:rounded-md dark:border-[1.5px] dark:border-border">
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

      {/* Compact dropdown — shown at the same breakpoint. */}
      <div className="hidden justify-center max-[1280px]:flex">
        <Select
          value={activeTab?.path}
          onValueChange={(value) => void navigate(`${basePath}/${value}`)}
        >
          <SelectTrigger
            aria-label="Select agent tab"
            className="pointer-events-auto h-10 w-48 bg-background px-3 text-body-sm font-medium tracking-wide [&>span]:flex [&>span]:items-center [&>span]:gap-2"
          >
            <SelectValue placeholder="Select tab" />
          </SelectTrigger>
          <SelectContent align="center">
            {TABS.map((tab) => (
              <SelectItem key={tab.path} value={tab.path}>
                <span className="inline-flex items-center gap-2">
                  <tab.icon className="size-4 shrink-0" />
                  {tab.label}
                </span>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </nav>
  );
}
