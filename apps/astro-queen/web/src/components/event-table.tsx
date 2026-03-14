import { Fragment, useState } from "react";
import { formatDateTime } from "@/lib/utils";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type CloudEventRow = any;

function EventRow({ event: ev }: { event: CloudEventRow }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <>
      <tr
        className="border-b border-comb-light hover:bg-glass-light cursor-pointer"
        onClick={() => setExpanded((e) => !e)}
      >
        <td className="px-2 py-1 text-amber whitespace-nowrap">{ev.type}</td>
        <td className="px-2 py-1 font-mono text-[10px] whitespace-nowrap">{ev.subject}</td>
        <td className="px-2 py-1 text-muted-foreground whitespace-nowrap">{ev.source}</td>
        <td className="px-2 py-1 text-muted-foreground whitespace-nowrap">{ev.time ? formatDateTime(ev.time) : "-"}</td>
      </tr>
      {expanded && (
        <tr className="border-b border-comb-light bg-glass-light">
          <td colSpan={4} className="px-2 py-1.5">
            <div className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-[10px]">
              <span className="text-muted-foreground">ID</span>
              <span className="font-mono">{ev.id}</span>
              {ev.data && typeof ev.data === "object" && Object.entries(ev.data as Record<string, unknown>).map(([k, v]) => (
                <Fragment key={k}>
                  <span className="text-muted-foreground">{k}</span>
                  <span className="font-mono">{typeof v === "object" ? JSON.stringify(v) : String(v)}</span>
                </Fragment>
              ))}
              {ev.ingestedAt && (
                <>
                  <span className="text-muted-foreground">ingested</span>
                  <span>{formatDateTime(ev.ingestedAt)}</span>
                </>
              )}
            </div>
          </td>
        </tr>
      )}
    </>
  );
}

export function EventTable({ events }: { events: CloudEventRow[] }) {
  return (
    <div className="overflow-x-auto rounded-lg glass">
      <table className="w-full text-[11px]">
        <thead>
          <tr className="border-b border-glass-border-honey glass-subtle">
            <th className="px-2 py-1 text-left font-medium text-muted-foreground">Type</th>
            <th className="px-2 py-1 text-left font-medium text-muted-foreground">Subject</th>
            <th className="px-2 py-1 text-left font-medium text-muted-foreground">Source</th>
            <th className="px-2 py-1 text-left font-medium text-muted-foreground">Time</th>
          </tr>
        </thead>
        <tbody>
          {events.map((ev, i) => (
            <EventRow key={ev.id || i} event={ev} />
          ))}
          {events.length === 0 && (
            <tr><td colSpan={4} className="px-2 py-3 text-center text-muted-foreground text-[10px]">No events</td></tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
