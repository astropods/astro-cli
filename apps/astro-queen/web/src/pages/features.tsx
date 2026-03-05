import { useState, useCallback } from "react";
import { useFeatures, useCreateFeature, useDeleteFeature } from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { Plus, Trash2, CheckCircle2 } from "lucide-react";
import { formatDateTime } from "@/lib/utils";
import { useOpenAPISchema } from "@/api/openmeter";
import { extractSchema, validateAgainstSchema, formatErrors, SCHEMA_REFS } from "@/lib/schemas";
import type { Feature } from "@/types/openmeter";

export function FeaturesPage() {
  const { data, isLoading, error } = useFeatures();
  const createMut = useCreateFeature();
  const deleteMut = useDeleteFeature();
  const [showCreate, setShowCreate] = useState(false);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Features</h2>
        <Button size="sm" onClick={() => setShowCreate(true)}><Plus className="size-3.5" /> Create Feature</Button>
      </div>

      <CreateFeatureDialog open={showCreate} onOpenChange={setShowCreate} onSubmit={(body) => createMut.mutate(body, { onSuccess: () => setShowCreate(false) })} isPending={createMut.isPending} />

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}
      {data && (
        <div className="overflow-x-auto rounded-lg glass">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-glass-border-honey glass-subtle">
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Key</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Name</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Meter Slug</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Archived</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Created</th>
                <th className="px-4 py-2 text-left font-medium text-muted-foreground">Actions</th>
              </tr>
            </thead>
            <tbody>
              {data.map((f) => (
                <tr key={f.id || f.key} className="border-b border-comb-light hover:bg-glass-light">
                  <td className="px-4 py-2 font-mono text-xs text-amber">{f.key}</td>
                  <td className="px-4 py-2">{f.name}</td>
                  <td className="px-4 py-2 text-muted-foreground">{f.meterSlug || "-"}</td>
                  <td className="px-4 py-2 text-muted-foreground">{f.archivedAt ? "Yes" : "No"}</td>
                  <td className="px-4 py-2 text-muted-foreground">{f.createdAt ? formatDateTime(f.createdAt) : "-"}</td>
                  <td className="px-4 py-2">
                    <Button variant="ghost" size="icon-xs" onClick={() => { if (confirm(`Delete feature "${f.key}"?`)) deleteMut.mutate(f.id || f.key); }}><Trash2 className="size-3 text-red-500" /></Button>
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

function CreateFeatureDialog({ open, onOpenChange, onSubmit, isPending }: { open: boolean; onOpenChange: (open: boolean) => void; onSubmit: (body: Partial<Feature>) => void; isPending: boolean }) {
  const { data: spec, isLoading: schemaLoading } = useOpenAPISchema();
  const [form, setForm] = useState({ key: "", name: "", meterSlug: "" });
  const [rawJson, setRawJson] = useState("");
  const [mode, setMode] = useState<string>("pretty");
  const [validated, setValidated] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);
  const set = (k: string, v: string) => { setForm((f) => ({ ...f, [k]: v })); setValidated(false); setErrors([]); };

  const handleTabChange = useCallback((tab: string) => {
    if (tab === "json") { setRawJson(JSON.stringify(Object.fromEntries(Object.entries(form).filter(([, v]) => v !== "")), null, 2)); }
    else { try { const p = JSON.parse(rawJson); setForm({ key: p.key ?? "", name: p.name ?? "", meterSlug: p.meterSlug ?? "" }); } catch {} }
    setMode(tab); setValidated(false); setErrors([]);
  }, [form, rawJson]);

  const getBody = (): Record<string, unknown> | null => {
    if (mode === "json") { try { return JSON.parse(rawJson); } catch { setErrors(["Invalid JSON"]); return null; } }
    return Object.fromEntries(Object.entries(form).filter(([, v]) => v !== ""));
  };

  const handleValidate = () => {
    const body = getBody();
    if (!body || !spec) return;
    const schema = extractSchema(spec, SCHEMA_REFS.FeatureCreate);
    const { valid, errors: valErrors } = validateAgainstSchema(schema, body);
    if (valid) { setValidated(true); setErrors([]); }
    else { setValidated(false); setErrors(formatErrors(valErrors)); }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm" showCloseButton={false}>
        <Tabs value={mode} onValueChange={handleTabChange}>
          <div className="flex items-start justify-between">
            <DialogHeader>
              <DialogTitle>Create Feature</DialogTitle>
              <DialogDescription>Define a new feature linked to a meter.</DialogDescription>
            </DialogHeader>
            <TabsList className="h-6 p-0.5">
              <TabsTrigger value="pretty" className="h-5 px-2 text-[10px]">Pretty</TabsTrigger>
              <TabsTrigger value="json" className="h-5 px-2 text-[10px]">JSON</TabsTrigger>
            </TabsList>
          </div>
          <TabsContent value="pretty" className="mt-3">
            <FieldGroup className="gap-4">
              <Field className="gap-1.5"><FieldLabel className="text-xs">Key <span className="text-destructive">*</span></FieldLabel><Input className="h-8 text-xs" value={form.key} onChange={(e) => set("key", e.target.value)} /></Field>
              <Field className="gap-1.5"><FieldLabel className="text-xs">Name <span className="text-destructive">*</span></FieldLabel><Input className="h-8 text-xs" value={form.name} onChange={(e) => set("name", e.target.value)} /></Field>
              <Field className="gap-1.5"><FieldLabel className="text-xs">Meter Slug</FieldLabel><Input className="h-8 text-xs" value={form.meterSlug} onChange={(e) => set("meterSlug", e.target.value)} /></Field>
            </FieldGroup>
          </TabsContent>
          <TabsContent value="json" className="mt-3">
            <Textarea value={rawJson} onChange={(e) => { setRawJson(e.target.value); setValidated(false); setErrors([]); }} className="min-h-32 font-mono text-xs" />
          </TabsContent>
        </Tabs>
        {errors.length > 0 && <ul className="text-xs text-destructive space-y-0.5">{errors.map((e, i) => <li key={i}>{e}</li>)}</ul>}
        {validated && <p className="flex items-center gap-1 text-xs text-green-600"><CheckCircle2 className="size-3" /> Validation passed</p>}
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>Cancel</Button>
          {validated ? <Button size="sm" onClick={() => { const b = getBody(); if (b) onSubmit(b as Partial<Feature>); }} disabled={isPending}>Create</Button> : <Button size="sm" variant="secondary" onClick={handleValidate} disabled={schemaLoading}>{schemaLoading ? "Loading..." : "Validate"}</Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
