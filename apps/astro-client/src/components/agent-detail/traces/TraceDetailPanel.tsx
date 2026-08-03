import { useEffect, useMemo, useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import type { TraceEntry } from "@/lib/api";
import { useObservabilityTraceDetail } from "@/api/queries/observability";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { TracePanelTitle, TraceNavButtons } from "./detail/TracePanelHeader";
import { SidePanel } from "@/components/ui/side-panel";
import { TraceMetaGrid } from "./detail/TraceMetaGrid";
import { TraceTabs, type TraceTab } from "./detail/TraceTabs";
import { TraceOverviewTab } from "./detail/TraceOverviewTab";
import { ObservationTree } from "./detail/ObservationTree";
import { ObservationDetail } from "./detail/ObservationDetail";

export interface TraceDetailPanelProps {
  deploymentId: string;
  trace: TraceEntry;
  account: string;
  onClose: () => void;
  canGoPrev?: boolean;
  canGoNext?: boolean;
  onNavigate?: (dir: "prev" | "next") => void;
  /** When true the panel renders content for full-width mode (wider grids). */
  expanded?: boolean;
  /** When provided, the header renders a maximize/restore button. */
  onToggleExpanded?: () => void;
}

export function TraceDetailPanel({
  deploymentId,
  trace,
  account,
  onClose,
  canGoPrev,
  canGoNext,
  onNavigate,
  expanded,
  onToggleExpanded,
}: TraceDetailPanelProps) {
  const [tab, setTab] = useState<TraceTab>("trace");
  const [selectedObsId, setSelectedObsId] = useState<string | null>(null);

  // The panel keeps the URL in sync with the open trace (?trace=<id>), so the
  // current location is already a shareable deep link. Copy it verbatim.
  const { copy, copied } = useCopyToClipboard();
  const handleShare = async () => {
    if (await copy(window.location.href)) toast("Link copied");
    else toast.error("Couldn't copy link");
  };

  const { data, isLoading, isError } = useObservabilityTraceDetail(
    deploymentId,
    trace.trace_id,
  );

  // Reset transient state when navigating to a different trace.
  useEffect(() => {
    setSelectedObsId(null);
    setTab("trace");
  }, [trace.trace_id]);

  const observations = useMemo(() => data?.observations ?? [], [data?.observations]);
  const scores = useMemo(() => data?.scores ?? [], [data?.scores]);

  const selectedObservation = useMemo(
    () => observations.find((o) => o.id === selectedObsId) ?? null,
    [observations, selectedObsId],
  );

  // Header / metadata tiles use the list entry as the canonical source — it
  // always has the right values and lets the panel render instantly without
  // waiting for the detail fetch. The detail endpoint enriches with body
  // content, tags, session, and the rest.
  const traceForDisplay = useMemo(() => {
    const base = data?.trace;
    return {
      trace_id: trace.trace_id,
      name: trace.name,
      timestamp: trace.timestamp,
      latency_ms: trace.latency_ms,
      total_cost: trace.total_cost ?? base?.total_cost ?? 0,
      input: base?.input,
      output: base?.output,
      session_id: base?.session_id,
      user_id: base?.user_id ?? trace.user_id,
      user_details: base?.user_details ?? trace.user_details,
      tags: base?.tags,
      metadata: base?.metadata,
      environment: base?.environment,
      release: base?.release,
      version: base?.version,
    };
  }, [data?.trace, trace]);

  const totalTokens = useMemo(() => {
    if (trace.total_tokens) return trace.total_tokens;
    return observations.reduce((sum, o) => sum + (o.usage?.total ?? 0), 0);
  }, [trace.total_tokens, observations]);

  return (
    <SidePanel
      background="card"
      ariaLabel="Trace details"
      closeLabel="Close trace"
      onClose={onClose}
      expanded={expanded}
      onToggleExpanded={onToggleExpanded}
      title={
        <TracePanelTitle
          timestamp={traceForDisplay.timestamp}
          traceId={traceForDisplay.trace_id}
          onShare={handleShare}
          shareCopied={copied}
        />
      }
      headerActions={
        <TraceNavButtons canGoPrev={canGoPrev} canGoNext={canGoNext} onNavigate={onNavigate} />
      }
    >
      <TraceMetaGrid
        latencyMs={traceForDisplay.latency_ms}
        totalCost={traceForDisplay.total_cost}
        totalTokens={totalTokens}
      />

      <TraceTabs
        active={tab}
        onChange={(t) => {
          setTab(t);
          if (t === "trace") setSelectedObsId(null);
        }}
        observationCount={observations.length}
      />

      <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
        {tab === "trace" && (
          <TraceOverviewTab
            trace={traceForDisplay}
            scores={scores}
            account={account}
          />
        )}

        {tab === "tree" && (
          <>
            {isLoading && (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="size-5 animate-spin text-muted-foreground" />
              </div>
            )}
            {isError && !isLoading && (
              <p className="px-2 py-4 text-body-sm text-muted-foreground">
                Failed to load observations.
              </p>
            )}
            {!isLoading && !isError &&
              (expanded ? (
                <div className="flex h-full min-h-0 gap-4">
                  <div className="min-w-0 flex-1 overflow-y-auto pr-1">
                    <ObservationTree
                      observations={observations}
                      selectedId={selectedObsId}
                      onSelect={setSelectedObsId}
                      showTimeline
                    />
                  </div>
                  <div className="min-w-0 flex-[1.4] overflow-y-auto border-l border-border/40 pl-4">
                    {selectedObservation ? (
                      <ObservationDetail
                        deploymentId={deploymentId}
                        observation={selectedObservation}
                        onBack={() => setSelectedObsId(null)}
                      />
                    ) : (
                      <p className="px-2 py-8 text-center text-body-sm text-muted-foreground">
                        Select an observation to view its details.
                      </p>
                    )}
                  </div>
                </div>
              ) : selectedObservation ? (
                <ObservationDetail
                  deploymentId={deploymentId}
                  observation={selectedObservation}
                  onBack={() => setSelectedObsId(null)}
                />
              ) : (
                <ObservationTree
                  observations={observations}
                  selectedId={selectedObsId}
                  onSelect={setSelectedObsId}
                />
              ))}
          </>
        )}
      </div>
    </SidePanel>
  );
}
