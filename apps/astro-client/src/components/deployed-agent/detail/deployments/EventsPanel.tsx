import { TriangleAlert, CheckCircle2 } from "lucide-react";
import { cn } from "@/lib/utils";
import { formatDurationMs } from "./history/utils";
import type { K8sEvent } from "@/lib/api";

function formatTimeAgo(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime();
  const label = formatDurationMs(ms);
  return label === "—" ? label : `${label} ago`;
}

interface EventsPanelProps {
  events: K8sEvent[];
}

export function EventsPanel({ events }: EventsPanelProps) {
  return (
    <div className="bg-stone-50">
      {events.length === 0 ? (
        <div className="p-4 font-mono text-mono-sm text-faint-foreground">No events</div>
      ) : (
        events.map((evt, i) => {
          const isWarning = evt.type === "Warning";
          return (
            <div
              key={`${evt.reason}-${evt.object_name}-${evt.last_timestamp}-${i}`}
              className={cn(
                "flex items-start gap-2.5 px-4 py-[9px]",
                i < events.length - 1 && "border-b border-border",
                isWarning && "bg-amber-50 dark:bg-amber-950/20",
              )}
            >
              {isWarning ? (
                <TriangleAlert size={14} className="shrink-0 mt-0.5 text-amber-600" />
              ) : (
                <CheckCircle2 size={14} className="shrink-0 mt-0.5 text-green-600" />
              )}
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="font-mono text-mono-sm font-medium text-foreground">
                    {evt.reason}
                  </span>
                  <span className="font-mono text-mono-sm text-faint-foreground">
                    {evt.object_kind}/{evt.object_name}
                  </span>
                  {evt.count > 1 && (
                    <span className="font-mono text-mono-sm text-faint-foreground">
                      ×{evt.count}
                    </span>
                  )}
                </div>
                <p className="font-sans text-body-sm text-muted-foreground mt-0.5 mb-0 line-clamp-2">
                  {evt.message}
                </p>
              </div>
              <span className="font-mono text-mono-sm text-faint-foreground shrink-0 mt-0.5">
                {formatTimeAgo(evt.last_timestamp)}
              </span>
            </div>
          );
        })
      )}
    </div>
  );
}
