import type { ReactNode } from "react";
import type { WorkloadDetail } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Bot, Activity, Database, Brain, Box, Download } from "lucide-react";
import { useLastErrorLog } from "@/api/queries/deployments";
import { classify, brandIconId, type Role } from "./classify";
import { getIntegrationIcon } from "@/lib/integrationIcons";
import { DeploymentAvatar } from "@/components/DeploymentAvatar";
import { Card } from "@/components/ui/card";

// "probing" is rendered while the runtime query is still in flight (record
// returned, runtime undefined). It's visually distinct from "pending"
// (which means K8s reported a starting pod) so the user can tell whether
// we're waiting on the cluster or whether the cluster is genuinely warming
// up a pod.
// "paused" is rendered when the parent deployment is in the paused/inactive
// state — every tile gets it regardless of K8s's own per-workload status,
// since "the whole agent is off" trumps any individual workload's idle/active
// reading (e.g. a CronJob still appearing Idle when nothing will actually fire).
export type PodStatus = "healthy" | "warning" | "unhealthy" | "pending" | "probing" | "paused";

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

/**
 * Visual styling for every PodStatus. Single source of truth — PodDetailPanel
 * imports this map directly rather than maintaining a parallel copy. Adding
 * a new status here automatically propagates everywhere the dot/glow/label
 * is rendered.
 */
export const POD_STATUS_STYLES: Record<PodStatus, { dot: string; glow: string; label: string }> = {
  healthy:   { dot: "bg-green-400",                   glow: "shadow-[0_0_6px_2px] shadow-green-400/50",  label: "Online" },
  warning:   { dot: "bg-amber-400",                   glow: "shadow-[0_0_6px_2px] shadow-amber-400/50",  label: "Degraded" },
  unhealthy: { dot: "bg-red-400",                     glow: "shadow-[0_0_6px_2px] shadow-red-400/50",    label: "Error" },
  pending:   { dot: "bg-blue-400",                    glow: "shadow-[0_0_6px_2px] shadow-blue-400/50",   label: "Starting" },
  probing:   { dot: "bg-muted-foreground/60 animate-pulse", glow: "",                                    label: "Probing" },
  paused:    { dot: "bg-stone-500",                   glow: "",                                          label: "Paused" },
};

export function resolvePodStatus(
  workload: WorkloadDetail | undefined,
  opts: { paused?: boolean; probing?: boolean },
): PodStatusInfo {
  if (opts.paused) return { status: "paused", label: POD_STATUS_STYLES.paused.label };
  if (opts.probing) return { status: "probing", label: POD_STATUS_STYLES.probing.label };
  return derivePodStatus(workload);
}

type IconComponent = React.ComponentType<{ className?: string }>;

const ROLE_ICONS: Record<Role, IconComponent> = {
  agent: Bot,
  knowledge: Database,
  model: Brain,
  integration: Box,
  ingestion: Download,
  collector: Activity,
  other: Box,
};

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
  /** Leading element rendered in place of the icon (e.g. the agent's avatar). */
  leading?: ReactNode;
  age?: string;
  warningMessage?: string | null;
  errorMessage?: string | null;
  className?: string;
  onClick?: () => void;
  selected?: boolean;
  dimmed?: boolean;
}

export function PodTileContent({ name, status = "pending", statusLabel, icon: Icon = Box, leading, age, warningMessage, errorMessage, className, onClick, selected, dimmed }: PodTileContentProps) {
  const styles = POD_STATUS_STYLES[status] ?? POD_STATUS_STYLES.pending;
  const { dot, glow } = styles;
  const label = statusLabel ?? styles.label;

  return (
    <div
      onClick={onClick}
      className={cn(
        "group max-w-[300px] drop-shadow-[0_2px_8px_rgba(0,0,0,0.12)] transition-[filter] duration-200 dark:drop-shadow-[0_2px_10px_rgba(0,0,0,0.4)]",
        onClick && "cursor-pointer",
        dimmed && "dark:brightness-[0.7]",
        className,
      )}
    >
      <Card
        className={cn(
          "relative overflow-hidden rounded-lg transition-colors duration-200",
          selected && "border-primary/60",
          !selected && onClick && "group-hover:border-primary/40",
        )}
      >
        <div
          className={cn(
            "pointer-events-none absolute inset-0 bg-primary/[0.04] transition-opacity duration-200 dark:bg-primary/[0.08]",
            selected ? "opacity-100" : onClick ? "opacity-0 group-hover:opacity-100" : "opacity-0",
          )}
        />
        <div className="relative">
          <div className="flex items-center gap-2.5 px-4 pt-3 pb-2">
            {leading ?? <Icon className="size-5 shrink-0 text-muted-foreground" />}
            <span className="text-base font-medium text-foreground">{name}</span>
            {age && <span className="ml-auto text-xs text-muted-foreground">{age}</span>}
          </div>
          <div className="flex items-center gap-1.5 px-4 pb-3">
            <span className={cn("size-1.5 shrink-0 rounded-full", dot, glow)} />
            <span className="text-xs text-muted-foreground">{label}</span>
          </div>
          {warningMessage && <TileNotice color="text-amber-400">{warningMessage}</TileNotice>}
          {errorMessage && <TileNotice color="text-red-400">{errorMessage}</TileNotice>}
        </div>
      </Card>
    </div>
  );
}

/** Connected pod tile — derives status and fetches error logs for unhealthy pods. */
interface PodTileProps {
  workload: WorkloadDetail | undefined;
  deploymentId: string;
  /**
   * True while the runtime query is in flight and we have no live state yet.
   * Renders a grey blinking "Probing" indicator instead of the K8s-derived
   * status so users can distinguish "we don't know yet" from "K8s says
   * starting". Once runtime returns, callers flip this to false and the
   * derived status takes over.
   */
  probing?: boolean;
  /**
   * True when the parent deployment is paused. Every tile reads "Paused"
   * regardless of the K8s-derived per-workload status, so the whole agent
   * being off trumps an individual CronJob still appearing Idle, etc.
   */
  paused?: boolean;
  /**
   * The parent deployment. When present, the agent tile renders the deployment's
   * avatar in place of the generic agent icon.
   */
  deployment?: { id: string; name: string; avatar_url?: string };
  className?: string;
  onClick?: () => void;
  selected?: boolean;
  dimmed?: boolean;
}

export function PodTile({ workload, deploymentId, probing, paused, deployment, className, onClick, selected, dimmed }: PodTileProps) {
  // Hooks must run unconditionally (rules-of-hooks). Pass safe defaults when
  // workload is missing so we can still bail out below — see PodGraph for the
  // root-cause fix; this is a defensive backstop for AnimatePresence exits and
  // query refetches that can briefly feed undefined through.
  const { status, label } = resolvePodStatus(workload, { paused, probing });
  const containerName = workload && status === "unhealthy" ? findUnhealthyContainer(workload) : "";
  const { data: errorLogs } = useLastErrorLog(
    deploymentId,
    workload?.name ?? "",
    containerName,
    !!workload && status === "unhealthy",
  );
  if (!workload) return null;
  const lastError = errorLogs?.[0]?.message ?? null;
  // "Restarting frequently" only applies to long-running pods that are
  // flapping (isFlapping → status="warning"). Job/CronJob also use the
  // "warning" status for Suspended state, but that has no restart-count
  // semantics — guarding on kind + a non-zero restart total prevents the
  // contradictory "Restarting frequently (0 restarts)" message from
  // appearing on a Suspended ingestion tile.
  const isLongRunning = workload.kind === "Deployment" || workload.kind === "StatefulSet";
  const totalRestarts = isLongRunning
    ? (workload.containers ?? []).reduce((sum, c) => sum + (c.restart_count ?? 0), 0)
    : 0;
  const warningMessage = status === "warning" && isLongRunning && totalRestarts > 0
    ? `Restarting frequently (${totalRestarts} restarts)`
    : null;

  // Hide ephemeral details (age, restart warnings, error logs) when the
  // tile is in a non-live state — they're either stale (paused) or
  // unknown (probing).
  const idle = paused || probing;
  // Agent tile → deployment avatar; knowledge/model/integration → brand icon
  // when shipped; otherwise the role icon (via PodTileContent's `icon`).
  const role = classify(workload.component, workload.kind);
  const brandId = brandIconId(role, workload.provider, workload.component);
  const leading =
    role === "agent" && deployment ? (
      <DeploymentAvatar deployment={deployment} size={20} className="size-5 shrink-0 rounded-sm" />
    ) : brandId ? (
      <span className="block size-5 shrink-0">{getIntegrationIcon(brandId)}</span>
    ) : undefined;
  return (
    <PodTileContent
      name={workload.component || workload.name}
      status={status}
      statusLabel={label}
      icon={ROLE_ICONS[role]}
      leading={leading}
      age={idle ? undefined : workload.age}
      warningMessage={idle ? null : warningMessage}
      errorMessage={idle ? null : lastError}
      className={className}
      onClick={onClick}
      selected={selected}
      dimmed={dimmed}
    />
  );
}
