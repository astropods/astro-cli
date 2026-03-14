import { useEvents, useIngestEvent } from "@/api/openmeter";
import { Skeleton } from "@/components/ui/skeleton";
import { SchemaFormPanel } from "@/components/schema-form-panel";
import { EventTable } from "@/components/event-table";

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
          {data && <EventTable events={data} />}
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
