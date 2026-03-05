import { useState } from "react";
import { useMeters, useCreateMeter, useDeleteMeter, useUpdateMeter, useQueryMeter } from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { Plus, Trash2, Search, X, Pencil } from "lucide-react";
import { formatDateTime } from "@/lib/utils";
import type { Meter } from "@/types/openmeter";

export function MetersPage() {
  const { data, isLoading, error } = useMeters();
  const createMut = useCreateMeter();
  const deleteMut = useDeleteMeter();
  const [showCreate, setShowCreate] = useState(false);
  const [queryMeter, setQueryMeter] = useState<Meter | null>(null);
  const [editMeter, setEditMeter] = useState<Meter | null>(null);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Meters</h2>
        <Button size="sm" onClick={() => setShowCreate(true)}>
          <Plus className="size-3.5" /> Create Meter
        </Button>
      </div>

      {showCreate && <CreateMeterForm onClose={() => setShowCreate(false)} onSubmit={(body) => createMut.mutate(body, { onSuccess: () => setShowCreate(false) })} isPending={createMut.isPending} />}
      {editMeter && <EditMeterForm meter={editMeter} onClose={() => setEditMeter(null)} />}
      {queryMeter && <QueryMeterForm meter={queryMeter} onClose={() => setQueryMeter(null)} />}

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Slug</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Name</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Event Type</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Aggregation</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Value Property</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Created</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.map((m) => (
                <tr key={m.id || m.slug} className="border-b border-comb-light hover:bg-glass-light">
                  <td className="px-4 py-2 font-mono text-xs text-amber">{m.slug}</td>
                  <td className="px-4 py-2">{m.name}</td>
                  <td className="px-4 py-2 text-muted-foreground">{m.eventType}</td>
                  <td className="px-4 py-2 text-muted-foreground">{m.aggregation}</td>
                  <td className="px-4 py-2 font-mono text-xs text-muted-foreground">{m.valueProperty || "-"}</td>
                  <td className="px-4 py-2 text-muted-foreground">{m.createdAt ? formatDateTime(m.createdAt) : "-"}</td>
                  <td className="px-4 py-2">
                    <div className="flex gap-1">
                      <Button variant="ghost" size="icon-xs" onClick={() => setQueryMeter(m)}>
                        <Search className="size-3" />
                      </Button>
                      <Button variant="ghost" size="icon-xs" onClick={() => setEditMeter(m)}>
                        <Pencil className="size-3" />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon-xs"
                        onClick={() => { if (confirm(`Delete meter "${m.slug}"?`)) deleteMut.mutate(m.id || m.slug); }}
                      >
                        <Trash2 className="size-3 text-red-500" />
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

const AGGREGATIONS = ["COUNT", "SUM", "UNIQUE_COUNT", "AVG", "MIN", "MAX", "LATEST"] as const;

function CreateMeterForm({ onClose, onSubmit, isPending }: { onClose: () => void; onSubmit: (body: Partial<Meter>) => void; isPending: boolean }) {
  const [form, setForm] = useState({ slug: "", name: "", description: "", eventType: "", aggregation: "COUNT", valueProperty: "", groupByKey: "", groupByValue: "" });
  const set = (k: string, v: string) => setForm((f) => ({ ...f, [k]: v }));

  const needsValue = form.aggregation !== "COUNT" && form.aggregation !== "UNIQUE_COUNT";

  return (
    <div className="rounded-lg glass-heavy p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-medium">Create Meter</h3>
        <Button variant="ghost" size="icon-xs" onClick={onClose}><X className="size-3.5" /></Button>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="mb-1 block text-xs text-muted-foreground">Slug <span className="text-red-500">*</span></label>
          <Input value={form.slug} onChange={(e) => set("slug", e.target.value)} placeholder="api_requests" />
          <p className="mt-0.5 text-[10px] text-muted-foreground">Lowercase, underscores only (e.g. api_requests)</p>
        </div>
        <div>
          <label className="mb-1 block text-xs text-muted-foreground">Name</label>
          <Input value={form.name} onChange={(e) => set("name", e.target.value)} placeholder="API Requests" />
          <p className="mt-0.5 text-[10px] text-muted-foreground">Human-readable display name</p>
        </div>
        <div>
          <label className="mb-1 block text-xs text-muted-foreground">Event Type <span className="text-red-500">*</span></label>
          <Input value={form.eventType} onChange={(e) => set("eventType", e.target.value)} placeholder="api.request" />
          <p className="mt-0.5 text-[10px] text-muted-foreground">CloudEvents type to match</p>
        </div>
        <div>
          <label className="mb-1 block text-xs text-muted-foreground">Aggregation <span className="text-red-500">*</span></label>
          <select
            value={form.aggregation}
            onChange={(e) => set("aggregation", e.target.value)}
            className="flex h-8 w-full rounded-md border border-glass-border-honey bg-glass-light backdrop-blur-sm px-3 py-1 text-sm text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-honey/40"
          >
            {AGGREGATIONS.map((a) => (
              <option key={a} value={a}>{a}</option>
            ))}
          </select>
          <p className="mt-0.5 text-[10px] text-muted-foreground">How event values are combined</p>
        </div>
        {needsValue && (
          <div>
            <label className="mb-1 block text-xs text-muted-foreground">Value Property <span className="text-red-500">*</span></label>
            <Input value={form.valueProperty} onChange={(e) => set("valueProperty", e.target.value)} placeholder="$.duration_ms" />
            <p className="mt-0.5 text-[10px] text-muted-foreground">JSONPath to the numeric value in event data</p>
          </div>
        )}
        <div>
          <label className="mb-1 block text-xs text-muted-foreground">Group By</label>
          <div className="flex gap-1">
            <Input value={form.groupByKey} onChange={(e) => set("groupByKey", e.target.value)} placeholder="key" className="flex-1" />
            <Input value={form.groupByValue} onChange={(e) => set("groupByValue", e.target.value)} placeholder="$.path" className="flex-1" />
          </div>
          <p className="mt-0.5 text-[10px] text-muted-foreground">Optional: group name and JSONPath (e.g. method / $.method)</p>
        </div>
        <div className="col-span-2">
          <label className="mb-1 block text-xs text-muted-foreground">Description</label>
          <Input value={form.description} onChange={(e) => set("description", e.target.value)} placeholder="Tracks API request count by endpoint" />
        </div>
      </div>
      <div className="mt-4 flex items-center gap-3">
        <Button size="sm" onClick={() => {
          const { groupByKey, groupByValue, ...rest } = form;
          const body: Record<string, unknown> = Object.fromEntries(
            Object.entries(rest).filter(([, v]) => v !== "")
          );
          if (groupByKey && groupByValue) {
            body.groupBy = { [groupByKey]: groupByValue };
          }
          onSubmit(body as Partial<Meter>);
        }} disabled={isPending || !form.slug || !form.eventType || (needsValue && !form.valueProperty)}>
          Create Meter
        </Button>
        <Button variant="ghost" size="sm" onClick={onClose}>Cancel</Button>
      </div>
    </div>
  );
}

function EditMeterForm({ meter, onClose }: { meter: Meter; onClose: () => void }) {
  const updateMut = useUpdateMeter();
  const [form, setForm] = useState({ name: meter.name, description: meter.description });
  const set = (k: string, v: string) => setForm((f) => ({ ...f, [k]: v }));

  return (
    <div className="rounded-lg glass-heavy p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-medium">Edit {meter.slug}</h3>
        <Button variant="ghost" size="icon-xs" onClick={onClose}><X className="size-3.5" /></Button>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Name" value={form.name} onChange={(v) => set("name", v)} />
        <Field label="Description" value={form.description} onChange={(v) => set("description", v)} />
      </div>
      <div className="mt-3">
        <Button size="sm" onClick={() => updateMut.mutate({ id: meter.id || meter.slug, body: form }, { onSuccess: onClose })} disabled={updateMut.isPending}>
          Save
        </Button>
      </div>
    </div>
  );
}

function QueryMeterForm({ meter, onClose }: { meter: Meter; onClose: () => void }) {
  const queryMut = useQueryMeter();
  const [body, setBody] = useState(JSON.stringify({ from: new Date(Date.now() - 86400000).toISOString(), to: new Date().toISOString(), windowSize: "HOUR" }, null, 2));

  return (
    <div className="rounded-lg glass-heavy p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-medium">Query {meter.slug}</h3>
        <Button variant="ghost" size="icon-xs" onClick={onClose}><X className="size-3.5" /></Button>
      </div>
      <Textarea value={body} onChange={(e) => setBody(e.target.value)} className="font-mono text-xs" />
      <div className="mt-2">
        <Button size="sm" onClick={() => { try { queryMut.mutate({ id: meter.id || meter.slug, body: JSON.parse(body) }); } catch {} }} disabled={queryMut.isPending}>
          Query
        </Button>
      </div>
      {queryMut.error && <p className="mt-2 text-xs text-destructive">{queryMut.error.message}</p>}
      {queryMut.data != null && (
        <pre className="mt-2 max-h-64 overflow-auto rounded glass-subtle p-2 text-xs text-foreground">
          {JSON.stringify(queryMut.data, null, 2)}
        </pre>
      )}
    </div>
  );
}

function Field({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  return (
    <div>
      <label className="mb-1 block text-xs text-muted-foreground">{label}</label>
      <Input value={value} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}
