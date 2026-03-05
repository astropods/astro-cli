import { useState, useCallback } from "react";
import { useEvents, useIngestEvent } from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { Plus, Send, CheckCircle2 } from "lucide-react";
import { formatDateTime } from "@/lib/utils";
import { useOpenAPISchema } from "@/api/openmeter";
import { extractSchema, validateAgainstSchema, formatErrors, SCHEMA_REFS } from "@/lib/schemas";

export function EventsPage() {
  const { data, isLoading, error } = useEvents();
  const ingestMut = useIngestEvent();
  const [showIngest, setShowIngest] = useState(false);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Events</h2>
        <Button size="sm" onClick={() => setShowIngest(true)}><Plus className="size-3.5" /> Ingest Event</Button>
      </div>

      <IngestDialog open={showIngest} onOpenChange={setShowIngest} onSubmit={(body) => ingestMut.mutate(body, { onSuccess: () => setShowIngest(false) })} isPending={ingestMut.isPending} error={ingestMut.error?.message} />

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

type EventForm = { type: string; subject: string; source: string; data: string };

function formToEvent(form: EventForm) {
  return { specversion: "1.0", id: crypto.randomUUID(), type: form.type, source: form.source, subject: form.subject, time: new Date().toISOString(), data: JSON.parse(form.data) };
}

function IngestDialog({ open, onOpenChange, onSubmit, isPending, error }: { open: boolean; onOpenChange: (open: boolean) => void; onSubmit: (body: unknown) => void; isPending: boolean; error?: string }) {
  const { data: spec, isLoading: schemaLoading } = useOpenAPISchema();
  const [form, setForm] = useState<EventForm>({ type: "", subject: "", source: "queen-ui", data: "{}" });
  const [rawJson, setRawJson] = useState("");
  const [mode, setMode] = useState<string>("pretty");
  const [validated, setValidated] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);
  const set = (k: string, v: string) => { setForm((f) => ({ ...f, [k]: v })); setValidated(false); setErrors([]); };

  const handleTabChange = useCallback((tab: string) => {
    if (tab === "json") { try { setRawJson(JSON.stringify(formToEvent(form), null, 2)); } catch { setRawJson("{}"); } }
    else { try { const p = JSON.parse(rawJson); setForm({ type: p.type ?? "", subject: p.subject ?? "", source: p.source ?? "queen-ui", data: JSON.stringify(p.data ?? {}, null, 2) }); } catch {} }
    setMode(tab); setValidated(false); setErrors([]);
  }, [form, rawJson]);

  const getBody = (): Record<string, unknown> | null => {
    if (mode === "json") { try { return JSON.parse(rawJson); } catch { setErrors(["Invalid JSON"]); return null; } }
    try { return formToEvent(form); } catch { setErrors(["Invalid data JSON"]); return null; }
  };

  const handleValidate = () => {
    const body = getBody();
    if (!body || !spec) return;
    const schema = extractSchema(spec, SCHEMA_REFS.Event);
    const { valid, errors: valErrors } = validateAgainstSchema(schema, body);
    if (valid) { setValidated(true); setErrors([]); }
    else { setValidated(false); setErrors(formatErrors(valErrors)); }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg" showCloseButton={false}>
        <Tabs value={mode} onValueChange={handleTabChange}>
          <div className="flex items-start justify-between">
            <DialogHeader>
              <DialogTitle>Ingest Event</DialogTitle>
              <DialogDescription>Send a CloudEvents-formatted event.</DialogDescription>
            </DialogHeader>
            <TabsList className="h-6 p-0.5">
              <TabsTrigger value="pretty" className="h-5 px-2 text-[10px]">Pretty</TabsTrigger>
              <TabsTrigger value="json" className="h-5 px-2 text-[10px]">JSON</TabsTrigger>
            </TabsList>
          </div>
          <TabsContent value="pretty" className="mt-3">
            <FieldGroup className="gap-4">
              <div className="grid grid-cols-3 gap-3">
                <Field className="gap-1.5"><FieldLabel className="text-xs">Type <span className="text-destructive">*</span></FieldLabel><Input className="h-8 text-xs" value={form.type} onChange={(e) => set("type", e.target.value)} /></Field>
                <Field className="gap-1.5"><FieldLabel className="text-xs">Subject <span className="text-destructive">*</span></FieldLabel><Input className="h-8 text-xs" value={form.subject} onChange={(e) => set("subject", e.target.value)} /></Field>
                <Field className="gap-1.5"><FieldLabel className="text-xs">Source</FieldLabel><Input className="h-8 text-xs" value={form.source} onChange={(e) => set("source", e.target.value)} /></Field>
              </div>
              <Field className="gap-1.5"><FieldLabel className="text-xs">Data (JSON)</FieldLabel><Textarea value={form.data} onChange={(e) => set("data", e.target.value)} className="min-h-20 font-mono text-xs" /></Field>
            </FieldGroup>
          </TabsContent>
          <TabsContent value="json" className="mt-3">
            <Textarea value={rawJson} onChange={(e) => { setRawJson(e.target.value); setValidated(false); setErrors([]); }} className="min-h-48 font-mono text-xs" />
          </TabsContent>
        </Tabs>
        {errors.length > 0 && <ul className="text-xs text-destructive space-y-0.5">{errors.map((e, i) => <li key={i}>{e}</li>)}</ul>}
        {error && <p className="text-xs text-destructive">{error}</p>}
        {validated && errors.length === 0 && <p className="flex items-center gap-1 text-xs text-green-600"><CheckCircle2 className="size-3" /> Validation passed</p>}
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>Cancel</Button>
          {validated ? <Button size="sm" onClick={() => { const b = getBody(); if (b) onSubmit(b); }} disabled={isPending}><Send className="size-3.5" /> Ingest</Button> : <Button size="sm" variant="secondary" onClick={handleValidate} disabled={schemaLoading}>{schemaLoading ? "Loading..." : "Validate"}</Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
