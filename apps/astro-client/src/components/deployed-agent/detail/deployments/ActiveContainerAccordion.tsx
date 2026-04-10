import { useEffect, useRef, useState } from "react";
import { ChevronRight, MoreVertical } from "lucide-react";
import { CheckIcon } from "@heroicons/react/24/outline";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useContainerSelection } from "@/hooks/use-container-selection";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { StatusIndicator } from "@/components/StatusIndicator";
import { statusVariant } from "./history/utils";
import { DomainsPanel } from "./DomainsPanel";
import { EnvVarsPanel } from "./EnvVarsPanel";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useRestartPod } from "@/api/queries/deployments";
import type { DeployHistoryStatus, MappedContainer, DomainUrl } from "./history/types";

export interface ActiveContainerAccordionProps {
  workloadName: string;
  podName?: string;
  title: string;
  isCompact?: boolean;
  urls?: DomainUrl[];
  readyText: string;
  uptime: string;
  containers: MappedContainer[];
  deploymentId: string;
  deploymentStatus: DeployHistoryStatus;
  isOpen: boolean;
  onToggle: () => void;
  /** True when a global (all-pods) restart is in progress — forces this accordion into restarting state. */
  isGloballyRestarting?: boolean;
  /** Called when this pod's local restart state changes, so the parent can reflect it in the status row. */
  onPodRestartStateChange?: (isRestarting: boolean) => void;
}

export function ActiveContainerAccordion({
  workloadName,
  podName,
  title,
  isCompact = false,
  urls,
  readyText,
  uptime,
  containers,
  deploymentId,
  deploymentStatus,
  isOpen,
  onToggle,
  isGloballyRestarting = false,
  onPodRestartStateChange,
}: ActiveContainerAccordionProps) {
  const [view, setView] = useState<"vars" | "domains">("vars");
  const restartMutation = useRestartPod();
  const [isLocallyRestarting, setIsLocallyRestarting] = useState(false);
  // canClear is unlocked 8s after mutation success — prevents instant clearing if pod restarts fast
  const [canClear, setCanClear] = useState(false);
  // isServiceRestarting persists until the pod is actually back up (not just during the HTTP call)
  const isServiceRestarting = isLocallyRestarting || isGloballyRestarting;

  const { selectedContainer, setSelectedContainer, activeContainer } = useContainerSelection(containers);

  const vars = activeContainer?.vars ?? [];
  const canShowVars = selectedContainer !== "collector";
  const canShowDomains = (urls ?? []).length > 0;
  const canExpand = canShowVars || canShowDomains;
  const totalContainers = containers.length;
  const readyContainers = containers.filter((c) => c.ready).length;
  const allReady = totalContainers > 0 && readyContainers === totalContainers;

  const clearRestarting = useRef(() => {});
  clearRestarting.current = () => {
    setIsLocallyRestarting(false);
    setCanClear(false);
    onPodRestartStateChange?.(false);
  };

  // Clear when canClear unlocked AND all containers are ready (pod is back up)
  useEffect(() => {
    if (!isLocallyRestarting || !canClear) return;
    if (allReady) clearRestarting.current();
  }, [isLocallyRestarting, canClear, allReady]);

  // Safety: clear after 60s regardless
  useEffect(() => {
    if (!isLocallyRestarting) return;
    const t = setTimeout(() => clearRestarting.current(), 60_000);
    return () => clearTimeout(t);
  }, [isLocallyRestarting]);

  const handleRestart = () => {
    if (!podName) return;
    setIsLocallyRestarting(true);
    setCanClear(false);
    onPodRestartStateChange?.(true);
    restartMutation.mutate({ deploymentId, podName }, {
      onSuccess: () => setTimeout(() => setCanClear(true), 8_000),
      onError: () => clearRestarting.current(),
    });
  };

  const effectiveView = (!canShowVars && view === "vars") ? "domains" : view;

  const effectiveStatus: DeployHistoryStatus = isServiceRestarting ? "restarting" : deploymentStatus;
  const isTransitioning = effectiveStatus === "deploying" || effectiveStatus === "undeploying" || effectiveStatus === "restarting" || effectiveStatus === "pausing" || effectiveStatus === "resuming";
  const variant = isTransitioning
    ? statusVariant(effectiveStatus)
    : allReady
      ? "success"
      : "warning";
  const isSpinning = isTransitioning;

  return (
    <div className="mb-1.5">
      <div
        className={cn(
          "flex gap-2 w-full px-3.5 py-2.5 border border-border text-left transition-[background] duration-150",
          canExpand ? "cursor-pointer" : "cursor-default",
          isOpen ? "rounded-t-sm border-b-muted bg-surface" : "rounded-sm bg-muted hover:bg-accent",
          isCompact ? "items-start flex-wrap" : "items-center flex-nowrap",
        )}
        onClick={canExpand ? onToggle : undefined}
      >
        {canExpand ? (
          <ChevronRight
            size={14}
            className={cn(
              "shrink-0 text-faint-foreground transition-transform duration-200",
              isOpen && "rotate-90",
            )}
          />
        ) : (
          <span className="w-3.5 shrink-0" />
        )}
        <StatusIndicator variant={variant} spinner={isSpinning} className="gap-0">
          {""}
        </StatusIndicator>
        <span className="font-sans text-heading-4 font-medium text-foreground" title={workloadName}>
          {title}
        </span>
        <span className="flex-1" />
        <span
          className={cn(
            "font-mono text-mono-sm text-foreground shrink-0",
            isCompact ? "ml-0 w-full" : "ml-2 w-auto",
          )}
        >
          <span className="inline-flex items-center gap-1">
            {readyText}
            <span>ready</span>
            {allReady ? <CheckIcon className="size-2.5" /> : null}
          </span>
          {" • "}
          {uptime}
        </span>

        {podName && (
          <div onClick={(e) => e.stopPropagation()}>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon-xs" aria-label={`Actions for ${title}`}>
                  <MoreVertical size={14} />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onClick={handleRestart} disabled={isServiceRestarting}>
                  {isServiceRestarting ? "Restarting…" : "Restart"}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        )}
      </div>

      {isOpen && canExpand && (
        <div className="border border-border border-t-0 rounded-b-sm overflow-hidden">
          <div className={cn("flex items-center bg-surface border-b border-border", isCompact ? "flex-wrap" : "flex-nowrap")}>
            {(["vars", "domains"] as const).map((v) =>
              (v === "domains" && !canShowDomains) ? null : (
                <button
                  key={v}
                  onClick={() => setView(v)}
                  className={cn(
                    "px-3.5 py-[7px] bg-transparent border-none cursor-pointer font-sans text-body capitalize border-b-2 transition-colors duration-100",
                    effectiveView === v
                      ? "font-medium text-foreground border-b-primary"
                      : "font-normal text-faint-foreground border-b-transparent",
                  )}
                >
                  {v === "vars" ? "Variables" : "Domains"}
                </button>
              ),
            )}
            {containers.length > 1 && (
              <div className={cn("ml-auto pr-2", isCompact && "pb-2")}>
                <Select value={selectedContainer} onValueChange={setSelectedContainer}>
                  <SelectTrigger className="h-7 w-auto min-w-[130px] px-3 font-sans text-body-sm bg-popover">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {containers.map((container) => (
                      <SelectItem key={container.name} value={container.name}>
                        {container.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
          </div>

          {effectiveView === "vars" && <EnvVarsPanel vars={vars} />}
          {effectiveView === "domains" && <DomainsPanel urls={urls ?? []} />}
        </div>
      )}
    </div>
  );
}
