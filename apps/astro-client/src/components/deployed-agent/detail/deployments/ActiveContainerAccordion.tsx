import { useEffect, useState } from "react";
import { ChevronRight, Copy, Check } from "lucide-react";
import { cn } from "@/lib/utils";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { useContainerSelection } from "@/hooks/use-container-selection";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { StatusIndicator } from "@/components/StatusIndicator";
import { statusVariant } from "./history/utils";
import { DomainsPanel } from "./DomainsPanel";
import { EnvVarsPanel } from "./EnvVarsPanel";
import { LogViewer } from "./LogViewer";
import type { DeployHistoryStatus, MappedContainer, DomainUrl } from "./history/types";

export interface ActiveContainerAccordionProps {
  workloadName: string;
  title: string;
  isCompact?: boolean;
  isAgentService?: boolean;
  url?: string;
  urls?: DomainUrl[];
  readyText: string;
  uptime: string;
  containers: MappedContainer[];
  deploymentId: string;
  deploymentStatus: DeployHistoryStatus;
  isOpen: boolean;
  onToggle: () => void;
}

export function ActiveContainerAccordion({
  workloadName,
  title,
  isCompact = false,
  isAgentService = false,
  url,
  urls,
  readyText,
  uptime,
  containers,
  deploymentId,
  deploymentStatus,
  isOpen,
  onToggle,
}: ActiveContainerAccordionProps) {
  const [view, setView] = useState<"logs" | "vars" | "domains">("logs");
  const { copy: copyPlayground, copied: copiedPlaygroundCommand } = useCopyToClipboard();

  const { selectedContainer, setSelectedContainer, activeContainer } = useContainerSelection(containers);

  const vars = activeContainer?.vars ?? [];
  const canShowVars = selectedContainer !== "collector";
  const canShowDomains = (urls ?? []).length > 0;
  const totalContainers = containers.length;
  const readyContainers = containers.filter((c) => c.ready).length;
  const allReady = totalContainers > 0 && readyContainers === totalContainers;

  useEffect(() => {
    if (!canShowVars && view === "vars") setView("logs");
    if (!canShowDomains && view === "domains") setView("logs");
  }, [canShowVars, canShowDomains, view]);

  const hasPublicUrl = !!url;
  const playgroundCommand = hasPublicUrl ? `ast playground ${url}` : "ast playground <deployment-url>";

  const handleCopyPlaygroundCommand = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!hasPublicUrl) return;
    void copyPlayground(playgroundCommand);
  };

  const variant = deploymentStatus === "deploying" || deploymentStatus === "undeploying"
    ? statusVariant(deploymentStatus)
    : allReady
      ? "success"
      : "warning";
  const isSpinning = deploymentStatus === "deploying" || deploymentStatus === "undeploying";

  return (
    <div className="mb-1.5">
      <div
        className={cn(
          "flex gap-2 w-full px-3.5 py-2.5 border border-border cursor-pointer text-left transition-[background] duration-150",
          isOpen ? "rounded-t-lg border-b-muted bg-surface" : "rounded-lg bg-muted hover:bg-stone-200",
          isCompact ? "items-start flex-wrap" : "items-center flex-nowrap",
        )}
        onClick={onToggle}
      >
        <ChevronRight
          size={14}
          className={cn(
            "shrink-0 text-faint-foreground transition-transform duration-200",
            isOpen && "rotate-90",
          )}
        />
        <StatusIndicator variant={variant} spinner={isSpinning} className="gap-0">
          {""}
        </StatusIndicator>
        <span className="font-sans text-heading-4 font-medium text-foreground" title={workloadName}>
          {title}
        </span>
        <span className="flex-1" />
        {isAgentService && (
          <div className="flex items-center gap-2 shrink-0 min-w-0" onClick={(e) => e.stopPropagation()}>
            <div className="flex items-center gap-1.5 min-w-0 text-foreground">
              <span className="font-sans text-body-sm whitespace-nowrap">
                To chat, run:
              </span>
              <button
                type="button"
                onClick={handleCopyPlaygroundCommand}
                title={hasPublicUrl ? playgroundCommand : "Public URL not available yet"}
                disabled={!hasPublicUrl}
                className={cn(
                  "inline-flex items-center gap-[5px] border border-border rounded px-2 py-0.5 bg-stone-200 cursor-pointer",
                  isCompact ? "max-w-[min(430px,50vw)]" : "max-w-[min(430px,55vw)]",
                  !hasPublicUrl && "cursor-not-allowed opacity-70",
                  !hasPublicUrl ? "text-faint-foreground" : copiedPlaygroundCommand ? "text-teal-600" : "text-muted-foreground",
                )}
              >
                <span className="font-mono text-mono-sm truncate">
                  {playgroundCommand}
                </span>
                {hasPublicUrl ? (copiedPlaygroundCommand ? <Check size={12} /> : <Copy size={12} />) : null}
              </button>
            </div>
          </div>
        )}
        <span
          className={cn(
            "font-mono text-mono-sm text-foreground shrink-0",
            isCompact ? "ml-0 w-full" : "ml-2 w-auto",
          )}
        >
          <span className="inline-flex items-center gap-1">
            {readyText}
            <span>ready</span>
            {allReady ? <Check size={10} /> : null}
          </span>
          {" • "}
          {uptime}
        </span>
      </div>

      {isOpen && (
        <div className="border border-border border-t-0 rounded-b-lg overflow-hidden">
          <div className={cn("flex items-center bg-surface border-b border-border", isCompact ? "flex-wrap" : "flex-nowrap")}>
            {(["logs", "vars", "domains"] as const).map((v) =>
              (v === "vars" && !canShowVars) || (v === "domains" && !canShowDomains) ? null : (
                <button
                  key={v}
                  onClick={() => setView(v)}
                  className={cn(
                    "px-3.5 py-[7px] bg-transparent border-none cursor-pointer font-sans text-body capitalize border-b-2 transition-colors duration-100",
                    view === v
                      ? "font-medium text-foreground border-b-teal-600"
                      : "font-normal text-faint-foreground border-b-transparent",
                  )}
                >
                  {v === "vars" ? "Variables" : v === "domains" ? "Domains" : "Logs"}
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

          {view === "vars" && <EnvVarsPanel vars={vars} />}
          {view === "domains" && <DomainsPanel urls={urls ?? []} />}
          {view === "logs" && (
            <LogViewer
              deploymentId={deploymentId}
              workloadName={workloadName}
              selectedContainer={selectedContainer}
              deploymentStatus={deploymentStatus}
              isOpen={isOpen}
              isCompact={isCompact}
            />
          )}
        </div>
      )}
    </div>
  );
}
