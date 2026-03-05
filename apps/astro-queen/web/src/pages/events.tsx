import { useState } from "react";
import { useEvents, useIngestEvent } from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Plus, Send } from "lucide-react";
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

      <IngestDialog
        open={showIngest}
        onOpenChange={setShowIngest}
        onSubmit={(body) => ingestMut.mutate(body, { onSuccess: () => setShowIngest(false) })}
        isPending={ingestMut.isPending}
        error={ingestMut.error?.message}
      />

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

function IngestDialog({
  open,
  onOpenChange,
  onSubmit,
  isPending,
  error,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
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
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Ingest Event</DialogTitle>
          <DialogDescription>Send a CloudEvents-formatted event.</DialogDescription>
        </DialogHeader>
        <div className="grid grid-cols-3 gap-3">
          <div>
            <Label>Type <span className="text-red-500">*</span></Label>
            <Input value={form.type} onChange={(e) => set("type", e.target.value)} />
          </div>
          <div>
            <Label>Subject <span className="text-red-500">*</span></Label>
            <Input value={form.subject} onChange={(e) => set("subject", e.target.value)} />
          </div>
          <div>
            <Label>Source</Label>
            <Input value={form.source} onChange={(e) => set("source", e.target.value)} />
          </div>
        </div>
        <div>
          <Label>Data (JSON)</Label>
          <Textarea
            value={form.data}
            onChange={(e) => set("data", e.target.value)}
            className="font-mono text-xs"
          />
        </div>
        {error && <p className="text-xs text-destructive">{error}</p>}
        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button size="sm" onClick={handleSubmit} disabled={isPending || !form.type || !form.subject}>
            <Send className="size-3.5" /> Ingest
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
