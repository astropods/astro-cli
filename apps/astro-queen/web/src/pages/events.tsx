import { useState } from "react";
import { useEvents, useIngestEvent } from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { Plus, X, Send } from "lucide-react";
import { formatDateTime } from "@/lib/utils";

export function EventsPage() {
  const { data, isLoading, error } = useEvents();
  const ingestMut = useIngestEvent();
  const [showIngest, setShowIngest] = useState(false);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Events</h2>
        <Button size="sm" onClick={() => setShowIngest(true)}>
          <Plus className="size-3.5" /> Ingest Event
        </Button>
      </div>

      {showIngest && (
        <IngestForm
          onClose={() => setShowIngest(false)}
          onSubmit={(body) => ingestMut.mutate(body, { onSuccess: () => setShowIngest(false) })}
          isPending={ingestMut.isPending}
          error={ingestMut.error?.message}
        />
      )}

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">ID</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Type</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Subject</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Source</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Time</th>
              </tr>
            </thead>
            <tbody>
              {data.map((ev, i) => (
                <tr key={ev.id || i} className="border-b border-comb-light hover:bg-glass-light">
                  <td className="px-4 py-2 font-mono text-xs text-muted-foreground">{ev.id}</td>
                  <td className="px-4 py-2 text-amber">{ev.type}</td>
                  <td className="px-4 py-2">{ev.subject}</td>
                  <td className="px-4 py-2 text-muted-foreground">{ev.source}</td>
                  <td className="px-4 py-2 text-muted-foreground">{ev.time ? formatDateTime(ev.time) : "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function IngestForm({
  onClose,
  onSubmit,
  isPending,
  error,
}: {
  onClose: () => void;
  onSubmit: (body: unknown) => void;
  isPending: boolean;
  error?: string;
}) {
  const [form, setForm] = useState({
    type: "",
    subject: "",
    source: "queen-ui",
    data: "{}",
  });
  const set = (k: string, v: string) => setForm((f) => ({ ...f, [k]: v }));

  const handleSubmit = () => {
    try {
      const event = {
        specversion: "1.0",
        id: crypto.randomUUID(),
        type: form.type,
        source: form.source,
        subject: form.subject,
        time: new Date().toISOString(),
        data: JSON.parse(form.data),
      };
      onSubmit(event);
    } catch {
      // invalid JSON
    }
  };

  return (
    <div className="rounded-lg glass-heavy p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-medium">Ingest Event</h3>
        <Button variant="ghost" size="icon-xs" onClick={onClose}>
          <X className="size-3.5" />
        </Button>
      </div>
      <div className="grid grid-cols-3 gap-3">
        <div>
          <label className="mb-1 block text-xs text-muted-foreground">Type *</label>
          <Input value={form.type} onChange={(e) => set("type", e.target.value)} />
        </div>
        <div>
          <label className="mb-1 block text-xs text-muted-foreground">Subject *</label>
          <Input value={form.subject} onChange={(e) => set("subject", e.target.value)} />
        </div>
        <div>
          <label className="mb-1 block text-xs text-muted-foreground">Source</label>
          <Input value={form.source} onChange={(e) => set("source", e.target.value)} />
        </div>
      </div>
      <div className="mt-3">
        <label className="mb-1 block text-xs text-muted-foreground">Data (JSON)</label>
        <Textarea
          value={form.data}
          onChange={(e) => set("data", e.target.value)}
          className="font-mono text-xs"
        />
      </div>
      {error && <p className="mt-2 text-xs text-destructive">{error}</p>}
      <div className="mt-3">
        <Button size="sm" onClick={handleSubmit} disabled={isPending || !form.type || !form.subject}>
          <Send className="size-3.5" /> Ingest
        </Button>
      </div>
    </div>
  );
}
