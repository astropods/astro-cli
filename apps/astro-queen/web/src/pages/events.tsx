import { useEvents, useIngestEvent } from "@/api/openmeter";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDateTime } from "@/lib/utils";
import { SchemaFormPanel } from "@/components/schema-form-panel";

export function EventsPage() {
  const { data, isLoading, error } = useEvents();
  const ingestMut = useIngestEvent();

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Events</h2>

      <div className="flex gap-4 items-start">
        <div className="min-w-0 flex-1">
          {isLoading && <Skeleton className="h-40 w-full" />}
          {error && <p className="text-destructive text-sm">{error.message}</p>}
          {data && (
            <div className="overflow-x-auto rounded-lg glass">
              <table className="w-full text-[11px] whitespace-nowrap">
                <thead>
                  <tr className="border-b border-glass-border-honey glass-subtle">
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">ID</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Type</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Subject</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Source</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Data</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Time</th>
                  </tr>
                </thead>
                <tbody>
                  {data.map((ev, i) => (
                    <tr key={ev.id || i} className="border-b border-comb-light hover:bg-glass-light">
                      <td className="px-2 py-0.5 font-mono text-xs text-muted-foreground">{ev.id}</td>
                      <td className="px-2 py-0.5 text-amber">{ev.type}</td>
                      <td className="px-2 py-0.5">{ev.subject}</td>
                      <td className="px-2 py-0.5 text-muted-foreground">{ev.source}</td>
                      <td className="px-2 py-0.5 text-muted-foreground font-mono text-[10px] max-w-[300px] truncate" title={ev.data ? JSON.stringify(ev.data) : ""}>
                        {ev.data ? JSON.stringify(ev.data) : "-"}
                      </td>
                      <td className="px-2 py-0.5 text-muted-foreground">{ev.time ? formatDateTime(ev.time) : "-"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        <SchemaFormPanel
          title="Ingest Event"
          description="Send a CloudEvents-formatted event."
          schemaRef="Event"
          submitLabel="Ingest"
          hiddenFields={["specversion"]}
          defaults={{ specversion: "1.0", id: crypto.randomUUID(), source: "queen-ui", time: new Date().toISOString() }}
          onSubmit={(body) => ingestMut.mutate(body)}
          isPending={ingestMut.isPending}
          error={ingestMut.error?.message}
        />
      </div>
    </div>
  );
}
