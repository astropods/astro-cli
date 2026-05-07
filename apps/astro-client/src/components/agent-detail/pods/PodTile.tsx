import type { WorkloadDetail } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Bot, Activity, Database, Brain, Box, Download } from "lucide-react";
import { ChatBubbleLeftRightIcon } from "@heroicons/react/24/outline";
import { useLastErrorLog } from "@/api/queries/deployments";
import { Squircle } from "../Squircle";

export type PodStatus = "healthy" | "warning" | "unhealthy" | "pending";

export interface PodStatusInfo {
  status: PodStatus;
  label: string;
}

export function derivePodStatus(workload: WorkloadDetail | undefined): PodStatusInfo {
  if (!workload) return { status: "pending", label: "Starting" };

  if (workload.kind === "Job") return deriveJobStatus(workload.status);
  if (workload.kind === "CronJob") return deriveCronJobStatus(workload.status);

  if (!workload.containers || workload.containers.length === 0) return { status: "pending", label: "Starting" };
  if (workload.containers.some((c) => c.state === "waiting" || c.state === "terminated")) return { status: "unhealthy", label: "Error" };
  if (workload.containers.every((c) => c.ready)) {
    if (isFlapping(workload)) return { status: "warning", label: "Degraded" };
    return { status: "healthy", label: "Online" };
  }
  return { status: "pending", label: "Starting" };
}

// Job status vocab from the server: Pending / Running / Succeeded / Failed.
function deriveJobStatus(status?: string): PodStatusInfo {
  switch (status) {
    case "Succeeded": return { status: "healthy", label: "Completed" };
    case "Failed":    return { status: "unhealthy", label: "Failed" };
    case "Running":   return { status: "healthy", label: "Running" };
    case "Pending":   return { status: "pending", label: "Pending" };
    default:          return { status: "pending", label: status || "Pending" };
  }
}

// CronJob status vocab from the server: Idle / Active / Suspended.
function deriveCronJobStatus(status?: string): PodStatusInfo {
  switch (status) {
    case "Active":    return { status: "healthy", label: "Running" };
    case "Idle":      return { status: "healthy", label: "Idle" };
    case "Suspended": return { status: "warning", label: "Suspended" };
    default:          return { status: "pending", label: status || "Idle" };
  }
}

/**
 * Detect if a pod is flapping — technically ready but restarting frequently.
 * Uses restart count relative to age to compute an approximate rate.
 */
function isFlapping(workload: WorkloadDetail): boolean {
  const totalRestarts = (workload.containers ?? []).reduce((sum, c) => sum + c.restart_count, 0);
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
  const containers = workload.containers ?? [];
  const unhealthy = containers.find(
    (c) => !c.ready || c.state === "waiting" || c.state === "terminated",
  );
  return unhealthy?.name ?? containers[0]?.name ?? "";
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

function getWorkloadIcon(workload: WorkloadDetail): IconComponent {
  if (workload.kind === "Job" || workload.kind === "CronJob") return Download;
  if (!workload.component) return Bot;
  return COMPONENT_ICONS[workload.component.toLowerCase()] ?? Box;
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
  /** Override the default status label (e.g. "Completed" for finished Jobs). */
  statusLabel?: string;
  icon?: IconComponent;
  age?: string;
  warningMessage?: string | null;
  errorMessage?: string | null;
  className?: string;
  onClick?: () => void;
  selected?: boolean;
  dimmed?: boolean;
}

export function PodTileContent({ name, status = "pending", statusLabel, icon: Icon = Box, age, warningMessage, errorMessage, className, onClick, selected, dimmed }: PodTileContentProps) {
  const styles = STATUS_STYLES[status] ?? STATUS_STYLES.pending;
  const { dot, glow } = styles;
  const label = statusLabel ?? styles.label;

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
  // Hooks must run unconditionally (rules-of-hooks). Pass safe defaults when
  // workload is missing so we can still bail out below — see PodGraph for the
  // root-cause fix; this is a defensive backstop for AnimatePresence exits and
  // query refetches that can briefly feed undefined through.
  const { status, label } = derivePodStatus(workload);
  const containerName = workload && status === "unhealthy" ? findUnhealthyContainer(workload) : "";
  const { data: errorLogs } = useLastErrorLog(
    deploymentId,
    workload?.name ?? "",
    containerName,
    !!workload && status === "unhealthy",
  );
  if (!workload) return null;
  const lastError = errorLogs?.[0]?.message ?? null;
  const warningMessage = status === "warning" && workload.containers
    ? `Restarting frequently (${workload.containers.reduce((sum, c) => sum + c.restart_count, 0)} restarts)`
    : null;

  return (
    <PodTileContent
      name={workload.component || workload.name}
      status={status}
      statusLabel={label}
      icon={getWorkloadIcon(workload)}
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
