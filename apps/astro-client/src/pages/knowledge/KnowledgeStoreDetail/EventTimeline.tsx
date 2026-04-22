import { useMemo } from "react";
import { ExclamationTriangleIcon, CheckCircleIcon } from "@heroicons/react/24/solid";
import type { KnowledgeStore, KnowledgeEvent } from "@/lib/api";
import { cn } from "@/lib/utils";

export function EventTimeline({ store }: { store: KnowledgeStore }) {
  const events: KnowledgeEvent[] = store.events ?? [];

  const groups = useMemo(() => {
    const result: { date: string; events: KnowledgeEvent[] }[] = [];
    for (const event of events) {
      const date = event.timestamp
        ? new Date(event.timestamp).toLocaleDateString("en-US", { month: "long", day: "numeric", year: "numeric" })
        : "Unknown date";
      const last = result[result.length - 1];
      if (last && last.date === date) {
        last.events.push(event);
      } else {
        result.push({ date, events: [event] });
      }
    }
    return result;
  }, [events]);

  if (events.length === 0) {
    return <p className="px-5 py-6 text-center text-body-sm text-muted-foreground">No events recorded</p>;
  }

  return (
    <>
      {groups.map((group, gi) => (
        <div key={group.date}>
          <div className={cn("px-5 py-1 border-b border-border", gi > 0 && "border-t")}>
            <span className="font-mono text-label uppercase tracking-[0.07em] text-faint-foreground">{group.date}</span>
          </div>
          {group.events.map((event, i) => (
            <div key={i} className="flex items-center gap-3 px-5 py-3 bg-white border-b border-border last:border-0">
              {event.timestamp && (
                <span className="w-14 shrink-0 whitespace-nowrap font-mono text-mono-sm tabular-nums text-muted-foreground">
                  {new Date(event.timestamp).toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit", hour12: true })}
                </span>
              )}
              <span className="flex-1 text-body-sm font-medium text-foreground">
                {event.reason}
                {event.count > 1 && <span className="ml-1 font-normal text-muted-foreground">×{event.count}</span>}
              </span>
              {event.type === "Warning" ? (
                <ExclamationTriangleIcon className="size-4 shrink-0 text-red-500" />
              ) : (
                <CheckCircleIcon className="size-4 shrink-0 text-teal-600" />
              )}
            </div>
          ))}
        </div>
      ))}
    </>
  );
}
