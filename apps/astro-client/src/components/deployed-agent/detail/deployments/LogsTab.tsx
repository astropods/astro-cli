import { useEffect, useMemo, useReducer, useState } from "react";
import { Plus, X } from "lucide-react";
import { CheckIcon } from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { LogViewer, type LogTimeRange } from "@/components/LogViewer";
import { useDeploymentLogs } from "@/api/queries/deployments";
import type { AgentDeployment, WorkloadDetail } from "@/lib/api";

interface LogTab {
  workloadName: string;
  containerName: string;
}

function tabKey(t: LogTab) {
  return `${t.workloadName}|${t.containerName}`;
}

/** "agent / app" for multi-container workloads, "agent" for single-container */
function tabLabel(wl: WorkloadDetail, containerName: string): string {
  const service = wl.component || wl.name;
  return (wl.containers ?? []).length <= 1 ? service : `${service} / ${containerName}`;
}

// ── Tab state ─────────────────────────────────────────────────────────────────

type TabState = { tabs: LogTab[]; activeKey: string | null };
type TabAction =
  | { type: "open"; tab: LogTab }
  | { type: "close"; key: string }
  | { type: "focus"; key: string }
  | { type: "reset" };

function tabReducer(state: TabState, action: TabAction): TabState {
  switch (action.type) {
    case "open": {
      const key = tabKey(action.tab);
      if (state.tabs.some((t) => tabKey(t) === key)) return { ...state, activeKey: key };
      return { tabs: [...state.tabs, action.tab], activeKey: key };
    }
    case "close": {
      const idx = state.tabs.findIndex((t) => tabKey(t) === action.key);
      if (idx === -1) return state;
      const next = state.tabs.filter((_, i) => i !== idx);
      if (state.activeKey !== action.key) return { tabs: next, activeKey: state.activeKey };
      const activeKey = next.length === 0 ? null : tabKey(next[Math.min(idx, next.length - 1)]);
      return { tabs: next, activeKey };
    }
    case "focus":
      return { ...state, activeKey: action.key };
    case "reset":
      return { tabs: [], activeKey: null };
  }
}

// ── Component ─────────────────────────────────────────────────────────────────

interface LogsTabProps {
  deployment: AgentDeployment;
  isCompact: boolean;
}

export function LogsTab({ deployment, isCompact }: LogsTabProps) {
  const workloads = deployment.workloads ?? [];

  const [{ tabs: openTabs, activeKey }, dispatch] = useReducer(tabReducer, { tabs: [], activeKey: null });
  const [timeRange, setTimeRange] = useState<LogTimeRange>("15m");

  // Reset open tabs when the deployment changes
  useEffect(() => {
    dispatch({ type: "reset" });
  }, [deployment.id]);

  const activeTab = useMemo(
    () => openTabs.find((t) => tabKey(t) === activeKey) ?? null,
    [openTabs, activeKey],
  );

  const { data: logsRaw, isLoading, isError } = useDeploymentLogs(
    deployment.id,
    activeTab?.workloadName ?? "",
    activeTab?.containerName ?? "",
    timeRange,
    { enabled: !!(activeTab?.workloadName && activeTab?.containerName) },
  );

  const logs = useMemo(
    () => (logsRaw ? logsRaw.split("\n").filter(Boolean) : []),
    [logsRaw],
  );

  if (workloads.length === 0) {
    return (
      <div className="flex items-center justify-center h-full font-mono text-mono-sm text-faint-foreground">
        No services available.
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full overflow-hidden">

      {/* ── Tab strip ────────────────────────────────────────────────── */}
      <div className="flex items-center bg-muted border-b border-border flex-shrink-0 px-[clamp(16px,4vw,108px)] py-0 gap-0">
        {openTabs.map((tab) => {
          const key = tabKey(tab);
          const wl = workloads.find((w) => w.name === tab.workloadName);
          const label = wl ? tabLabel(wl, tab.containerName) : tab.containerName;
          const isActive = key === activeKey;
          return (
            <button
              key={key}
              onClick={() => dispatch({ type: "focus", key })}
              className={cn(
                "group flex items-center gap-1.5 bg-transparent border-0 font-sans text-heading-4 py-[11px] px-4 border-b transition-colors duration-150 cursor-pointer whitespace-nowrap",
                isActive
                  ? "font-medium text-foreground border-b-[var(--color-teal-600)]"
                  : "font-normal text-faint-foreground border-b-transparent hover:text-foreground",
              )}
            >
              {label}
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={`Close ${label}`}
                onClick={(e) => { e.stopPropagation(); dispatch({ type: "close", key }); }}
                className={cn(
                  "rounded-full transition-colors",
                  isActive
                    ? "text-muted-foreground"
                    : "text-transparent group-hover:text-muted-foreground",
                )}
              >
                <X size={9} />
              </Button>
            </button>
          );
        })}

        {/* + Add tab */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              title="Open container logs in a new tab"
              className="ml-1 my-[9px] text-faint-foreground focus-visible:ring-0 focus-visible:border-transparent"
            >
              <Plus size={13} />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start" side="bottom" sideOffset={6} className="min-w-[200px]">
            {workloads.map((wl) => (
              <DropdownMenuGroup key={wl.name}>
                <DropdownMenuLabel className="px-2 py-1 font-mono text-mono-sm text-faint-foreground">
                  {wl.component || wl.name}
                </DropdownMenuLabel>
                {(wl.containers ?? []).map((c) => {
                  const alreadyOpen = openTabs.some(
                    (t) => t.workloadName === wl.name && t.containerName === c.name,
                  );
                  return (
                    <DropdownMenuItem
                      key={c.name}
                      onClick={() => dispatch({ type: "open", tab: { workloadName: wl.name, containerName: c.name } })}
                      className="cursor-pointer gap-2"
                    >
                      <span className="flex size-3.5 items-center justify-center shrink-0">
                        {alreadyOpen && <CheckIcon className="size-3 text-primary" />}
                      </span>
                      <span className={cn("font-mono text-mono-sm", alreadyOpen && "text-faint-foreground")}>
                        {c.name}
                      </span>
                    </DropdownMenuItem>
                  );
                })}
              </DropdownMenuGroup>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* ── Log viewer card ──────────────────────────────────────────── */}
      <div className="flex-1 min-h-0 overflow-hidden py-5 px-[clamp(16px,4vw,108px)]">
        {openTabs.length === 0 ? (
          <div className="flex items-center justify-center h-full font-mono text-mono-sm text-faint-foreground">
            Click <Plus size={12} className="mx-1.5 inline" /> to view logs for a container
          </div>
        ) : (
          <LogViewer
            logs={logs}
            isLoading={isLoading}
            isCompact={isCompact}
            timeRange={timeRange}
            onTimeRangeChange={setTimeRange}
            error={isError ? "Failed to load logs." : undefined}
          />
        )}
      </div>
    </div>
  );
}
