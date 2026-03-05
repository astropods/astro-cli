import { useState, useCallback } from "react";
import { useMeters, useCreateMeter, useDeleteMeter, useUpdateMeter, useQueryMeter } from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { Field, FieldLabel, FieldDescription, FieldGroup } from "@/components/ui/field";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Plus, Trash2, Search, Pencil, CheckCircle2 } from "lucide-react";
import { formatDateTime } from "@/lib/utils";
import { useOpenAPISchema } from "@/api/openmeter";
import { extractSchema, validateAgainstSchema, formatErrors, SCHEMA_REFS } from "@/lib/schemas";
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

      <CreateMeterDialog
        open={showCreate}
        onOpenChange={setShowCreate}
        onSubmit={(body) => createMut.mutate(body, { onSuccess: () => setShowCreate(false) })}
        isPending={createMut.isPending}
      />
      <EditMeterDialog
        meter={editMeter}
        onOpenChange={(open) => { if (!open) setEditMeter(null); }}
      />
      <QueryMeterDialog
        meter={queryMeter}
        onOpenChange={(open) => { if (!open) setQueryMeter(null); }}
      />

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
  );
}

const AGGREGATIONS = ["COUNT", "SUM", "UNIQUE_COUNT", "AVG", "MIN", "MAX", "LATEST"] as const;

type MeterForm = { slug: string; name: string; description: string; eventType: string; aggregation: string; valueProperty: string; groupByKey: string; groupByValue: string };
const EMPTY_FORM: MeterForm = { slug: "", name: "", description: "", eventType: "", aggregation: "COUNT", valueProperty: "", groupByKey: "", groupByValue: "" };

function formToJson(form: MeterForm): Record<string, unknown> {
  const { groupByKey, groupByValue, ...rest } = form;
  const body: Record<string, unknown> = Object.fromEntries(Object.entries(rest).filter(([, v]) => v !== ""));
  if (groupByKey && groupByValue) body.groupBy = { [groupByKey]: groupByValue };
  return body;
}

function jsonToForm(json: Record<string, unknown>): MeterForm {
  const groupBy = (json.groupBy ?? {}) as Record<string, string>;
  const entries = Object.entries(groupBy);
  return { slug: String(json.slug ?? ""), name: String(json.name ?? ""), description: String(json.description ?? ""), eventType: String(json.eventType ?? ""), aggregation: String(json.aggregation ?? "COUNT"), valueProperty: String(json.valueProperty ?? ""), groupByKey: entries[0]?.[0] ?? "", groupByValue: entries[0]?.[1] ?? "" };
}

function CreateMeterDialog({ open, onOpenChange, onSubmit, isPending }: { open: boolean; onOpenChange: (open: boolean) => void; onSubmit: (body: Partial<Meter>) => void; isPending: boolean }) {
  const { data: spec, isLoading: schemaLoading } = useOpenAPISchema();
  const [form, setForm] = useState<MeterForm>(EMPTY_FORM);
  const [rawJson, setRawJson] = useState("");
  const [mode, setMode] = useState<string>("pretty");
  const [validated, setValidated] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);
  const set = (k: string, v: string) => { setForm((f) => ({ ...f, [k]: v })); setValidated(false); setErrors([]); };
  const needsValue = form.aggregation !== "COUNT" && form.aggregation !== "UNIQUE_COUNT";

  const handleTabChange = useCallback((tab: string) => {
    if (tab === "json") { setRawJson(JSON.stringify(formToJson(form), null, 2)); }
    else { try { setForm(jsonToForm(JSON.parse(rawJson))); } catch {} }
    setMode(tab); setValidated(false); setErrors([]);
  }, [form, rawJson]);

  const getBody = (): Record<string, unknown> | null => {
    if (mode === "json") { try { return JSON.parse(rawJson); } catch { setErrors(["Invalid JSON"]); return null; } }
    return formToJson(form);
  };

  const handleValidate = () => {
    const body = getBody();
    if (!body || !spec) return;
    const schema = extractSchema(spec, SCHEMA_REFS.MeterCreate);
    const { valid, errors: valErrors } = validateAgainstSchema(schema, body);
    if (valid) { setValidated(true); setErrors([]); }
    else { setValidated(false); setErrors(formatErrors(valErrors)); }
  };

  const handleSubmit = () => {
    const body = getBody();
    if (body) onSubmit(body as Partial<Meter>);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl" showCloseButton={false}>
        <Tabs value={mode} onValueChange={handleTabChange}>
          <div className="flex items-start justify-between">
            <DialogHeader>
              <DialogTitle>Create Meter</DialogTitle>
              <DialogDescription>Define a new usage meter to track events.</DialogDescription>
            </DialogHeader>
            <TabsList className="h-6 p-0.5">
              <TabsTrigger value="pretty" className="h-5 px-2 text-[10px]">Pretty</TabsTrigger>
              <TabsTrigger value="json" className="h-5 px-2 text-[10px]">JSON</TabsTrigger>
            </TabsList>
          </div>
          <TabsContent value="pretty" className="mt-3">
            <FieldGroup className="gap-4">
              <div className="grid grid-cols-2 gap-x-3 gap-y-4">
                <Field className="gap-1.5">
                  <FieldLabel className="text-xs">Slug <span className="text-destructive">*</span></FieldLabel>
                  <Input className="h-8 text-xs" value={form.slug} onChange={(e) => set("slug", e.target.value)} placeholder="api_requests" />
                  <FieldDescription className="text-[10px]">Lowercase, underscores only</FieldDescription>
                </Field>
                <Field className="gap-1.5">
                  <FieldLabel className="text-xs">Name</FieldLabel>
                  <Input className="h-8 text-xs" value={form.name} onChange={(e) => set("name", e.target.value)} placeholder="API Requests" />
                </Field>
                <Field className="gap-1.5">
                  <FieldLabel className="text-xs">Event Type <span className="text-destructive">*</span></FieldLabel>
                  <Input className="h-8 text-xs" value={form.eventType} onChange={(e) => set("eventType", e.target.value)} placeholder="api.request" />
                </Field>
                <Field className="gap-1.5">
                  <FieldLabel className="text-xs">Aggregation <span className="text-destructive">*</span></FieldLabel>
                  <Select value={form.aggregation} onValueChange={(v) => set("aggregation", v)}>
                    <SelectTrigger size="sm" className="w-full text-xs"><SelectValue /></SelectTrigger>
                    <SelectContent>{AGGREGATIONS.map((a) => <SelectItem key={a} value={a}>{a}</SelectItem>)}</SelectContent>
                  </Select>
                </Field>
                {needsValue && (
                  <Field className="gap-1.5">
                    <FieldLabel className="text-xs">Value Property <span className="text-destructive">*</span></FieldLabel>
                    <Input className="h-8 text-xs" value={form.valueProperty} onChange={(e) => set("valueProperty", e.target.value)} placeholder="$.duration_ms" />
                  </Field>
                )}
                <Field className="gap-1.5">
                  <FieldLabel className="text-xs">Group By</FieldLabel>
                  <div className="flex gap-1.5">
                    <Input className="h-8 text-xs" value={form.groupByKey} onChange={(e) => set("groupByKey", e.target.value)} placeholder="key" />
                    <Input className="h-8 text-xs" value={form.groupByValue} onChange={(e) => set("groupByValue", e.target.value)} placeholder="$.path" />
                  </div>
                </Field>
              </div>
              <Field className="gap-1.5">
                <FieldLabel className="text-xs">Description</FieldLabel>
                <Input className="h-8 text-xs" value={form.description} onChange={(e) => set("description", e.target.value)} placeholder="Tracks API request count by endpoint" />
              </Field>
            </FieldGroup>
          </TabsContent>
          <TabsContent value="json" className="mt-3">
            <Textarea value={rawJson} onChange={(e) => { setRawJson(e.target.value); setValidated(false); setErrors([]); }} className="min-h-48 font-mono text-xs" />
          </TabsContent>
        </Tabs>
        {errors.length > 0 && (
          <ul className="text-xs text-destructive space-y-0.5">
            {errors.map((e, i) => <li key={i}>{e}</li>)}
          </ul>
        )}
        {validated && errors.length === 0 && (
          <p className="flex items-center gap-1 text-xs text-green-600"><CheckCircle2 className="size-3" /> Validation passed</p>
        )}
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>Cancel</Button>
          {validated ? (
            <Button size="sm" onClick={handleSubmit} disabled={isPending}>Create Meter</Button>
          ) : (
            <Button size="sm" variant="secondary" onClick={handleValidate} disabled={schemaLoading}>{schemaLoading ? "Loading..." : "Validate"}</Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
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
              <Field className="gap-1.5"><FieldLabel className="text-xs">Name</FieldLabel><Input className="h-8 text-xs" value={form.name} onChange={(e) => set("name", e.target.value)} /></Field>
              <Field className="gap-1.5"><FieldLabel className="text-xs">Description</FieldLabel><Input className="h-8 text-xs" value={form.description} onChange={(e) => set("description", e.target.value)} /></Field>
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
