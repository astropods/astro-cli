import type { WorkloadDetail } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Bot, Activity, Database, Brain, Box } from "lucide-react";
import { ChatBubbleLeftRightIcon } from "@heroicons/react/24/outline";
import { useLastErrorLog } from "@/api/queries/deployments";
import { Squircle } from "../Squircle";

export type PodStatus = "healthy" | "warning" | "unhealthy" | "pending";

export function derivePodStatus(workload: WorkloadDetail | undefined): PodStatus {
  if (!workload?.containers || workload.containers.length === 0) return "pending";
  if (workload.containers.some((c) => c.state === "waiting" || c.state === "terminated")) return "unhealthy";
  if (workload.containers.every((c) => c.ready)) {
    if (isFlapping(workload)) return "warning";
    return "healthy";
  }
  return "pending";
}

/**
 * Detect if a pod is flapping — technically ready but restarting frequently.
 * Uses restart count relative to age to compute an approximate rate.
 */
function isFlapping(workload: WorkloadDetail): boolean {
  const totalRestarts = workload.containers.reduce((sum, c) => sum + c.restart_count, 0);
  if (totalRestarts < 3) return false;

  const ageHours = parseAgeToHours(workload.age);
  if (ageHours <= 0) return totalRestarts >= 3;

  const restartsPerHour = totalRestarts / ageHours;
  return restartsPerHour >= 1;
}

/** Parse a Kubernetes age string like "3d", "12h", "45m", "30s" to hours. */
function parseAgeToHours(age?: string): number {
  if (!age) return 0;
  const match = age.match(/^(\d+)([dhms])$/);
  if (!match) return 0;
  const value = parseInt(match[1], 10);
  switch (match[2]) {
    case "d": return value * 24;
    case "h": return value;
    case "m": return value / 60;
    case "s": return value / 3600;
    default: return 0;
  }
}

function findUnhealthyContainer(workload: WorkloadDetail): string {
  const unhealthy = workload.containers.find(
    (c) => !c.ready || c.state === "waiting" || c.state === "terminated",
  );
  return unhealthy?.name ?? workload.containers[0]?.name ?? "";
}

const STATUS_STYLES: Record<PodStatus, { dot: string; glow: string; label: string }> = {
  healthy: {
    dot: "bg-green-400",
    glow: "shadow-[0_0_6px_2px] shadow-green-400/50",
    label: "Online",
  },
  warning: {
    dot: "bg-amber-400",
    glow: "shadow-[0_0_6px_2px] shadow-amber-400/50",
    label: "Degraded",
  },
  unhealthy: {
    dot: "bg-red-400",
    glow: "shadow-[0_0_6px_2px] shadow-red-400/50",
    label: "Error",
  },
  pending: {
    dot: "bg-blue-400",
    glow: "shadow-[0_0_6px_2px] shadow-blue-400/50",
    label: "Starting",
  },
};

type IconComponent = React.ComponentType<{ className?: string }>;

const COMPONENT_ICONS: Record<string, IconComponent> = {
  agent: Bot,
  collector: Activity,
  messaging: ChatBubbleLeftRightIcon,
  redis: Database,
  postgres: Database,
  qdrant: Database,
  neo4j: Database,
  ollama: Brain,
};

function getComponentIcon(component?: string): IconComponent {
  if (!component) return Bot;
  return COMPONENT_ICONS[component.toLowerCase()] ?? Box;
}

function TileNotice({ color, children }: { color: string; children: React.ReactNode }) {
  return (
    <div className="px-4 pb-3">
      <p className={cn("line-clamp-2 font-mono text-xs", color)}>{children}</p>
    </div>
  );
}

/** Presentational pod tile — no data fetching. */
export interface PodTileContentProps {
  name: string;
  status: PodStatus;
  icon?: IconComponent;
  age?: string;
  warningMessage?: string | null;
  errorMessage?: string | null;
  className?: string;
  onClick?: () => void;
  selected?: boolean;
  dimmed?: boolean;
}

export function PodTileContent({ name, status = "pending", icon: Icon = Box, age, warningMessage, errorMessage, className, onClick, selected, dimmed }: PodTileContentProps) {
  const { dot, glow, label } = STATUS_STYLES[status] ?? STATUS_STYLES.pending;

  return (
    <Squircle className={cn("max-w-[300px]", className)} onClick={onClick} selected={selected} dimmed={dimmed}>
      <div className="flex items-center gap-2.5 px-4 pt-3 pb-2">
        <Icon className="size-5 shrink-0 text-muted-foreground" />
        <span className="text-base font-medium text-foreground">
          {name}
        </span>
        {age && (
          <span className="ml-auto text-xs text-muted-foreground">
            {age}
          </span>
        )}
      </div>
      <div className="flex items-center gap-1.5 px-4 pb-3">
        <span className={cn("size-1.5 shrink-0 rounded-full", dot, glow)} />
        <span className="text-xs text-muted-foreground">{label}</span>
      </div>
      {warningMessage && <TileNotice color="text-amber-400">{warningMessage}</TileNotice>}
      {errorMessage && <TileNotice color="text-red-400">{errorMessage}</TileNotice>}
    </Squircle>
  );
}

/** Connected pod tile — derives status and fetches error logs for unhealthy pods. */
interface PodTileProps {
  workload: WorkloadDetail | undefined;
  deploymentId: string;
  className?: string;
  onClick?: () => void;
  selected?: boolean;
  dimmed?: boolean;
}

export function PodTile({ workload, deploymentId, className, onClick, selected, dimmed }: PodTileProps) {
  // Defense in depth: PodGraph also guards against stale-positions vs new
  // workloads, but render order during AnimatePresence exits or query refetches
  // can still feed `undefined` here briefly. Bail out so the page doesn't crash.
  if (!workload) return null;
  const status = derivePodStatus(workload);
  const containerName = status === "unhealthy" ? findUnhealthyContainer(workload) : "";
  const { data: errorLogs } = useLastErrorLog(
    deploymentId,
    workload.name,
    containerName,
    status === "unhealthy",
  );
  const lastError = errorLogs?.[0]?.message ?? null;

  const totalRestarts = workload.containers.reduce((sum, c) => sum + c.restart_count, 0);
  const warningMessage = status === "warning"
    ? `Restarting frequently (${totalRestarts} restarts)`
    : null;

  return (
    <PodTileContent
      name={workload.component || workload.name}
      status={status}
      icon={getComponentIcon(workload.component)}
      age={workload.age}
      warningMessage={warningMessage}
      errorMessage={lastError}
      className={className}
      onClick={onClick}
      selected={selected}
      dimmed={dimmed}
    />
  );
}
