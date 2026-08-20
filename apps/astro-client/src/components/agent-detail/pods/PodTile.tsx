import type { ReactNode } from "react";
import type { WorkloadDetail, ContainerStatus } from "@/lib/api";
import { cn } from "@/lib/utils";
import { AlertCircle, Bot, Activity, Database, Brain, Box, Download, TriangleAlert } from "lucide-react";
import { useLastErrorLog } from "@/api/queries/deployments";
import { classify, brandIconId, type Role } from "./classify";
import { getIntegrationIcon } from "@/lib/integrationIcons";
import { DeploymentAvatar } from "@/components/DeploymentAvatar";
import { Card } from "@/components/ui/card";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import { ContainerLogErrorProbe, firstContainerError, useContainerErrors } from "./use-container-log-errors";

// "probing" is rendered while the runtime query is still in flight (record
// returned, runtime undefined). It's visually distinct from "pending"
// (which means K8s reported a starting pod) so the user can tell whether
// we're waiting on the cluster or whether the cluster is genuinely warming
// up a pod.
// "paused" is rendered when the parent deployment is in the paused/inactive
// state — every tile gets it regardless of K8s's own per-workload status,
// since "the whole agent is off" trumps any individual workload's idle/active
// reading (e.g. a CronJob still appearing Idle when nothing will actually fire).
export type PodStatus = "healthy" | "warning" | "unhealthy" | "pending" | "probing" | "paused" | "suspended";

export interface PodStatusInfo {
  status: PodStatus;
  label: string;
}

// The server humanizes a benignly-starting container (ContainerCreating /
// PodInitializing) to this message, so it is not a failure.
export const STARTING_UP_MESSAGE = "Starting up";

// A "Waiting"/"Terminated" container is a real problem unless it is just
// starting up, so normal startup does not render as an error.
export function isContainerProblem(container: ContainerStatus): boolean {
  const s = container.state?.toLowerCase();
  if (s !== "waiting" && s !== "terminated") return false;
  return container.message !== STARTING_UP_MESSAGE;
}

export function derivePodStatus(workload: WorkloadDetail | undefined): PodStatusInfo {
  if (!workload) return { status: "pending", label: "Starting" };

  if (workload.kind === "Job") return deriveJobStatus(workload.status);
  if (workload.kind === "CronJob") return deriveCronJobStatus(workload.status);

  if (!workload.containers || workload.containers.length === 0) return { status: "pending", label: "Starting" };
  if (workload.containers.some(isContainerProblem)) return { status: "unhealthy", label: "Error" };
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
    (c) => !c.ready || isContainerProblem(c),
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
  suspended: { dot: "bg-amber-500",                   glow: "",                                          label: "Suspended" },
};

export function resolvePodStatus(
  workload: WorkloadDetail | undefined,
  opts: { paused?: boolean; suspended?: boolean; probing?: boolean },
): PodStatusInfo {
  // Outranks paused: both mean zero replicas, but only a pause is undoable.
  if (opts.suspended) return { status: "suspended", label: POD_STATUS_STYLES.suspended.label };
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

const STORAGE_WARN_PCT = 80;

const STORAGE_GRADIENT =
  "linear-gradient(90deg, var(--color-muted-foreground) 0%, var(--color-muted-foreground) 64%, var(--color-yellow-400) 82%, var(--color-red-400) 98%)";
const STORAGE_FULL_FILL = "linear-gradient(90deg, var(--color-red-400), var(--color-red-400))";

/** Compact "used/capacity" readout sharing one unit (e.g. "8.6/10 GB"). */
function formatStoragePair(usedBytes: number, capacityBytes: number): string {
  const units: [string, number][] = [
    ["GB", 1024 ** 3],
    ["MB", 1024 ** 2],
    ["KB", 1024],
  ];
  const [unit, div] = units.find(([, d]) => capacityBytes >= d) ?? ["B", 1];
  const fmt = (n: number) => {
    const v = n / div;
    return Number.isInteger(v) ? `${v}` : v.toFixed(1);
  };
  return `${fmt(usedBytes)}/${fmt(capacityBytes)} ${unit}`;
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
  /** Leading element rendered in place of the icon (e.g. the agent's avatar). */
  leading?: ReactNode;
  age?: string;
  warningMessage?: string | null;
  errorMessage?: string | null;
  /** Show a small alert icon when the container's logs contain errors or
   *  warnings, even if its runtime status looks healthy. */
  logIssue?: "error" | "warning" | null;
  /** Persistent-volume usage; when set (capacity > 0) the tile shows a fill bar. */
  storage?: { usedBytes: number; capacityBytes: number };
  className?: string;
  onClick?: () => void;
  selected?: boolean;
  dimmed?: boolean;
}

/** Error/warning icon shown on the tile when a live container is logging
 *  issues. Error uses the error icon + red; warning uses the warning icon +
 *  amber. Wrapped in its own TooltipProvider so the label shows instantly. */
function LogIssueIndicator({ severity }: { severity: "error" | "warning" }) {
  const isError = severity === "error";
  const Icon = isError ? AlertCircle : TriangleAlert;
  const label = isError ? "Errors found in logs" : "Warnings found in logs";
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span role="img" aria-label={label} className="flex items-center">
            <Icon className={cn("size-4", isError ? "text-red-400 dark:text-red-400" : "text-amber-400 dark:text-amber-400")} />
          </span>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

function StorageWarning({ pct }: { pct: number }) {
  const full = pct >= 100;
  const free = Math.max(1, Math.round(100 - pct));
  const label = full ? "Storage full" : `Low storage — ${free}% free`;
  const Icon = full ? AlertCircle : TriangleAlert;
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span role="img" aria-label={label} className="flex items-center">
            <Icon className={cn("size-3.5", full ? "text-red-400 dark:text-red-400" : "text-yellow-400 dark:text-yellow-400")} />
          </span>
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

export function PodTileContent({ name, status = "pending", statusLabel, icon: Icon = Box, leading, age, warningMessage, errorMessage, logIssue, storage, className, onClick, selected, dimmed }: PodTileContentProps) {
  const styles = POD_STATUS_STYLES[status] ?? POD_STATUS_STYLES.pending;
  const { dot, glow } = styles;
  const label = statusLabel ?? styles.label;

  const showStorage = !!storage && storage.capacityBytes > 0;
  const storagePct = showStorage
    ? Math.min(100, Math.max(0, (storage.usedBytes / storage.capacityBytes) * 100))
    : 0;

  return (
    <div
      onClick={onClick}
      className={cn(
        "group max-w-[300px] drop-shadow-[0_2px_8px_rgba(0,0,0,0.12)] dark:drop-shadow-[0_2px_10px_rgba(0,0,0,0.4)]",
        onClick && "cursor-pointer",
        className,
      )}
    >
      <Card
        className={cn(
          "relative overflow-hidden rounded-lg transition-colors duration-200",
          dimmed && "border-border/60",
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
        <div className={cn("relative transition-opacity duration-200", dimmed && "opacity-70")}>
          <div className="flex items-center gap-2.5 px-4 pt-3 pb-2">
            {leading ?? <Icon className="size-5 shrink-0 text-muted-foreground" />}
            <span className="text-base font-medium text-foreground">{name}</span>
            {(logIssue || age) && (
              <div className="ml-auto flex items-center gap-2">
                {age && <span className="text-xs text-muted-foreground">{age}</span>}
                {logIssue && <LogIssueIndicator severity={logIssue} />}
              </div>
            )}
          </div>
          <div className="flex items-center gap-1.5 px-4 pb-3">
            <span className={cn("size-1.5 shrink-0 rounded-full", dot, glow)} />
            <span className="text-xs text-muted-foreground">{label}</span>
          </div>
          {showStorage && (
            <div className="px-4 pb-3">
              <div className="mb-1 flex items-center justify-between gap-3 text-xs text-muted-foreground">
                <span>Storage</span>
                <span className="flex items-center gap-1.5">
                  <span className="text-foreground/80">
                    {formatStoragePair(storage.usedBytes, storage.capacityBytes)}
                  </span>
                  {storagePct >= STORAGE_WARN_PCT && <StorageWarning pct={storagePct} />}
                </span>
              </div>
              {/* -mx offsets the 1px border + 2px padding so the track aligns flush with the text above. */}
              <div className="-mx-[3px] rounded-[4px] border border-foreground/15 p-0.5">
                <div
                  className="relative h-1 w-full overflow-hidden rounded-[2px] bg-foreground/10"
                  role="progressbar"
                  aria-label="Storage used"
                  aria-valuenow={Math.round(storagePct)}
                  aria-valuemin={0}
                  aria-valuemax={100}
                >
                  {/* Full-width gradient revealed by clip-path, so color maps to absolute usage (solid red when full). */}
                  <div
                    className="absolute inset-0 rounded-[2px] transition-[clip-path] duration-300"
                    style={{
                      backgroundImage: storagePct >= 100 ? STORAGE_FULL_FILL : STORAGE_GRADIENT,
                      clipPath: `inset(0 ${100 - storagePct}% 0 0)`,
                    }}
                  />
                </div>
              </div>
            </div>
          )}
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
  /** True when billing stopped the deployment. Outranks paused. */
  suspended?: boolean;
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

export function PodTile({ workload, deploymentId, probing, paused, suspended, deployment, className, onClick, selected, dimmed }: PodTileProps) {
  // Hooks must run unconditionally (rules-of-hooks). Pass safe defaults when
  // workload is missing so we can still bail out below — see PodGraph for the
  // root-cause fix; this is a defensive backstop for AnimatePresence exits and
  // query refetches that can briefly feed undefined through.
  const { status, label } = resolvePodStatus(workload, { paused, suspended, probing });
  const containerName = workload && status === "unhealthy" ? findUnhealthyContainer(workload) : "";
  const { data: errorLogs } = useLastErrorLog(
    deploymentId,
    workload?.name ?? "",
    containerName,
    !!workload && status === "unhealthy",
  );
  const { byContainer, report } = useContainerErrors();
  if (!workload) return null;
  // Prefer the container's own failure explanation (ImagePullBackOff produces no logs).
  const statusError =
    status === "unhealthy"
      ? (workload.containers ?? []).find((c) => c.name === containerName)?.message ?? null
      : null;
  const lastError = statusError ?? errorLogs?.[0]?.message ?? null;
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
  const idle = paused || suspended || probing;
  // Agent tile → deployment avatar; knowledge/model/integration → brand icon
  // when shipped; otherwise the role icon (via PodTileContent's `icon`).
  const role = classify(workload.component, workload.kind);
  const brandId = brandIconId(role, workload.provider, workload.component);

  // Storage bar: knowledge tiles with live metrics only, hidden while idle.
  const storage =
    role === "knowledge" &&
    !idle &&
    typeof workload.storage_used_bytes === "number" &&
    typeof workload.storage_capacity_bytes === "number" &&
    workload.storage_capacity_bytes > 0
      ? { usedBytes: workload.storage_used_bytes, capacityBytes: workload.storage_capacity_bytes }
      : undefined;
  const leading =
    role === "agent" && deployment ? (
      <DeploymentAvatar deployment={deployment} size={20} className="size-5 shrink-0 rounded-sm" />
    ) : brandId ? (
      <span className="block size-5 shrink-0">{getIntegrationIcon(brandId)}</span>
    ) : undefined;

  // Surface a log-error indicator for a live, healthy-looking long-running pod
  // that is still logging errors. Unhealthy pods already show the last error
  // message above, so we only probe the non-unhealthy case to avoid a duplicate
  // signal. One probe per container keeps the hook count stable.
  const probeContainers =
    isLongRunning && !idle && status !== "unhealthy" ? workload.containers ?? [] : [];
  const hasLogErrors = !!firstContainerError(byContainer, probeContainers.map((c) => c.name));

  return (
    <>
      {probeContainers.map((c) => (
        <ContainerLogErrorProbe
          key={c.name}
          deploymentId={deploymentId}
          workloadName={workload.name}
          container={c.name}
          onResult={report}
        />
      ))}
      <PodTileContent
        name={workload.component || workload.name}
        status={status}
        statusLabel={label}
        icon={ROLE_ICONS[role]}
        leading={leading}
        age={idle ? undefined : workload.age}
        warningMessage={idle ? null : warningMessage}
        errorMessage={idle ? null : lastError}
        logIssue={hasLogErrors ? "error" : null}
        storage={storage}
        className={className}
        onClick={onClick}
        selected={selected}
        dimmed={dimmed}
      />
    </>
  );
}
