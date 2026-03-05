import { useState } from "react";
import { useMeters, useCreateMeter, useDeleteMeter, useUpdateMeter, useQueryMeter } from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Trash2, Search, Pencil, CheckCircle2 } from "lucide-react";
import { formatDateTime } from "@/lib/utils";
import { validateAgainstSchema, formatErrors } from "@/lib/schemas";
import { SchemaFormPanel } from "@/components/schema-form-panel";
import type { Meter } from "@/types/openmeter";

export function MetersPage() {
  const { data, isLoading, error } = useMeters();
  const createMut = useCreateMeter();
  const deleteMut = useDeleteMeter();
  const [queryMeter, setQueryMeter] = useState<Meter | null>(null);
  const [editMeter, setEditMeter] = useState<Meter | null>(null);

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Meters</h2>

      <EditMeterDialog
        meter={editMeter}
        onOpenChange={(open) => { if (!open) setEditMeter(null); }}
      />
      <QueryMeterDialog
        meter={queryMeter}
        onOpenChange={(open) => { if (!open) setQueryMeter(null); }}
      />

      <div className="flex gap-4 items-start">
        <div className="min-w-0 flex-1">
          {isLoading && <Skeleton className="h-40 w-full" />}
          {error && <p className="text-destructive text-sm">{error.message}</p>}
          {data && (
            <div className="overflow-x-auto rounded-lg glass">
              <table className="w-full text-[11px] whitespace-nowrap">
                <thead>
                  <tr className="border-b border-glass-border-honey glass-subtle">
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Slug</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Name</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Event Type</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Aggregation</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Value Property</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Created</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {data.map((m) => (
                    <tr key={m.id || m.slug} className="border-b border-comb-light hover:bg-glass-light">
                      <td className="px-2 py-0.5 font-mono text-xs text-amber">{m.slug}</td>
                      <td className="px-2 py-0.5">{m.name}</td>
                      <td className="px-2 py-0.5 text-muted-foreground">{m.eventType}</td>
                      <td className="px-2 py-0.5 text-muted-foreground">{m.aggregation}</td>
                      <td className="px-2 py-0.5 font-mono text-xs text-muted-foreground">{m.valueProperty || "-"}</td>
                      <td className="px-2 py-0.5 text-muted-foreground">{m.createdAt ? formatDateTime(m.createdAt) : "-"}</td>
                      <td className="px-2 py-0.5">
                        <div className="flex gap-1">
                          <Button variant="ghost" size="icon-xs" onClick={() => setQueryMeter(m)}>
                            <Search className="size-3" />
                          </Button>
                          <Button variant="ghost" size="icon-xs" onClick={() => setEditMeter(m)}>
                            <Pencil className="size-3" />
                          </Button>
                          <Button variant="ghost" size="icon-xs" onClick={() => { if (confirm(`Delete meter "${m.slug}"?`)) deleteMut.mutate(m.id || m.slug); }}>
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

        <SchemaFormPanel
          title="Create Meter"
          description="Define a new usage meter to track events."
          schemaRef="MeterCreate"
          submitLabel="Create Meter"
          onSubmit={(body) => createMut.mutate(body as Partial<Meter>)}
          isPending={createMut.isPending}
        />
      </div>
    </div>
  );
}

function EditMeterDialog({ meter, onOpenChange }: { meter: Meter | null; onOpenChange: (open: boolean) => void }) {
  const updateMut = useUpdateMeter();
  const [form, setForm] = useState({ name: "", description: "" });
  const [rawJson, setRawJson] = useState("");
  const [mode, setMode] = useState<string>("pretty");
  const [validated, setValidated] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);
  const set = (k: string, v: string) => { setForm((f) => ({ ...f, [k]: v })); setValidated(false); setErrors([]); };

  const prevSlug = useState<string | null>(null);
  if (meter && meter.slug !== prevSlug[0]) {
    prevSlug[1](meter.slug);
    setForm({ name: meter.name, description: meter.description });
    setRawJson(JSON.stringify({ name: meter.name, description: meter.description }, null, 2));
    setMode("pretty"); setValidated(false); setErrors([]);
  }

  const handleTabChange = (tab: string) => {
    if (tab === "json") { setRawJson(JSON.stringify(form, null, 2)); }
    else { try { setForm(JSON.parse(rawJson)); } catch {} }
    setMode(tab); setValidated(false); setErrors([]);
  };

  const getBody = (): Record<string, unknown> | null => {
    if (mode === "json") { try { return JSON.parse(rawJson); } catch { setErrors(["Invalid JSON"]); return null; } }
    return form;
  };

  const handleValidate = () => {
    const body = getBody();
    if (!body) return;
    const meterUpdateSchema = { type: "object", additionalProperties: false, properties: { name: { type: "string", maxLength: 256 }, description: { type: "string", maxLength: 1024 } } };
    const { valid, errors: valErrors } = validateAgainstSchema(meterUpdateSchema, body);
    if (valid) { setValidated(true); setErrors([]); }
    else { setValidated(false); setErrors(formatErrors(valErrors)); }
  };

  const handleSubmit = () => {
    const body = getBody();
    if (body) updateMut.mutate({ id: meter?.id || meter?.slug || "", body }, { onSuccess: () => onOpenChange(false) });
  };

  return (
    <Dialog open={!!meter} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm" showCloseButton={false}>
        <Tabs value={mode} onValueChange={handleTabChange}>
          <div className="flex items-start justify-between">
            <DialogHeader>
              <DialogTitle>Edit {meter?.slug}</DialogTitle>
              <DialogDescription>Update meter display name and description.</DialogDescription>
            </DialogHeader>
            <TabsList className="h-6 p-0.5">
              <TabsTrigger value="pretty" className="h-5 px-2 text-[10px]">Pretty</TabsTrigger>
              <TabsTrigger value="json" className="h-5 px-2 text-[10px]">JSON</TabsTrigger>
            </TabsList>
          </div>
          <TabsContent value="pretty" className="mt-3">
            <FieldGroup className="gap-4">
              <Field className="gap-1.5"><FieldLabel className="text-xs">Name</FieldLabel><Input className="h-7 text-xs" value={form.name} onChange={(e) => set("name", e.target.value)} /></Field>
              <Field className="gap-1.5"><FieldLabel className="text-xs">Description</FieldLabel><Input className="h-7 text-xs" value={form.description} onChange={(e) => set("description", e.target.value)} /></Field>
            </FieldGroup>
          </TabsContent>
          <TabsContent value="json" className="mt-3">
            <Textarea value={rawJson} onChange={(e) => { setRawJson(e.target.value); setValidated(false); setErrors([]); }} className="min-h-24 font-mono text-xs" />
          </TabsContent>
        </Tabs>
        {errors.length > 0 && <ul className="text-xs text-destructive space-y-0.5">{errors.map((e, i) => <li key={i}>{e}</li>)}</ul>}
        {validated && errors.length === 0 && <p className="flex items-center gap-1 text-xs text-green-600"><CheckCircle2 className="size-3" /> Validation passed</p>}
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>Cancel</Button>
          {validated ? <Button size="sm" onClick={handleSubmit} disabled={updateMut.isPending}>Save</Button> : <Button size="sm" variant="secondary" onClick={handleValidate}>Validate</Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function QueryMeterDialog({ meter, onOpenChange }: { meter: Meter | null; onOpenChange: (open: boolean) => void }) {
  const queryMut = useQueryMeter();
  const [body, setBody] = useState(JSON.stringify({ from: new Date(Date.now() - 86400000).toISOString(), to: new Date().toISOString(), windowSize: "HOUR" }, null, 2));

  return (
    <Dialog open={!!meter} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg" showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>Query {meter?.slug}</DialogTitle>
          <DialogDescription>Run a query against this meter.</DialogDescription>
        </DialogHeader>
        <Field className="gap-1.5">
          <FieldLabel className="text-xs">Query Body (JSON)</FieldLabel>
          <Textarea value={body} onChange={(e) => setBody(e.target.value)} className="min-h-24 font-mono text-xs" />
        </Field>
        {queryMut.error && <p className="text-xs text-destructive">{queryMut.error.message}</p>}
        {queryMut.data != null && (
          <pre className="max-h-64 overflow-auto rounded-md glass-subtle p-3 text-xs text-foreground">{JSON.stringify(queryMut.data, null, 2)}</pre>
        )}
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>Close</Button>
          <Button size="sm" onClick={() => { try { queryMut.mutate({ id: meter?.id || meter?.slug || "", body: JSON.parse(body) }); } catch {} }} disabled={queryMut.isPending}>Query</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
