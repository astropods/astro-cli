import { useState, useCallback } from "react";
import { useParams } from "react-router";
import {
  useCustomer, useCustomerEntitlements, useEntitlementValue, useEntitlementGrants,
  useCreateEntitlement, useDeleteEntitlement, useCreateGrant,
} from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Skeleton } from "@/components/ui/skeleton";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/components/ui/dialog";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import { Plus, Trash2, ChevronDown, CheckCircle2 } from "lucide-react";
import { formatDateTime } from "@/lib/utils";
import { useOpenAPISchema } from "@/api/openmeter";
import { extractSchema, validateAgainstSchema, formatErrors, SCHEMA_REFS } from "@/lib/schemas";
import type { Entitlement } from "@/types/openmeter";

export function CustomerDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: customer, isLoading, error } = useCustomer(id ?? "");
  const { data: entitlements } = useCustomerEntitlements(id ?? "");
  const [showCreateEnt, setShowCreateEnt] = useState(false);
  const createEnt = useCreateEntitlement();

  if (isLoading) return <Skeleton className="h-64 w-full" />;
  if (error) return <p className="text-destructive">Error: {error.message}</p>;
  if (!customer) return null;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">{customer.name}</h2>
        <p className="text-sm text-muted-foreground">{customer.id}</p>
      </div>
      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <InfoCard label="Email" value={customer.email} />
        <InfoCard label="Key" value={customer.key} />
        <InfoCard label="Currency" value={customer.currency} />
        <InfoCard label="Timezone" value={customer.timezone} />
      </div>
      <div>
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-sm font-medium text-muted-foreground">Entitlements</h3>
          <Button size="xs" variant="outline" onClick={() => setShowCreateEnt(true)}><Plus className="size-3" /> Add</Button>
        </div>
        <CreateEntitlementDialog open={showCreateEnt} onOpenChange={setShowCreateEnt} onSubmit={(body) => createEnt.mutate({ customerId: id!, body }, { onSuccess: () => setShowCreateEnt(false) })} isPending={createEnt.isPending} />
        {entitlements?.map((ent) => <EntitlementRow key={ent.id} customerId={id!} entitlement={ent} />)}
        {entitlements?.length === 0 && <p className="text-sm text-muted-foreground">No entitlements</p>}
      </div>
    </div>
  );
}

function InfoCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg glass px-3 py-2">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-0.5 truncate text-sm">{value || "-"}</p>
    </div>
  );
}

function EntitlementRow({ customerId, entitlement }: { customerId: string; entitlement: Entitlement }) {
  const deleteEnt = useDeleteEntitlement();
  const { data: value } = useEntitlementValue(customerId, entitlement.id);
  const { data: grants } = useEntitlementGrants(customerId, entitlement.id);
  const [showGrant, setShowGrant] = useState(false);
  const createGrant = useCreateGrant();

  return (
    <Collapsible className="mb-2 rounded-lg glass">
      <div className="flex items-center justify-between px-3 py-2">
        <CollapsibleTrigger className="flex items-center gap-1 text-sm hover:text-foreground">
          <ChevronDown className="size-3.5 text-muted-foreground" />
          <span className="font-medium text-amber">{entitlement.featureKey}</span>
          <span className="ml-2 text-xs text-muted-foreground">{entitlement.type}</span>
        </CollapsibleTrigger>
        <Button variant="ghost" size="icon-xs" onClick={() => deleteEnt.mutate({ customerId, entitlementId: entitlement.id })}><Trash2 className="size-3 text-red-500" /></Button>
      </div>
      <CollapsibleContent className="border-t border-glass-border-honey px-3 py-2">
        <div className="grid grid-cols-4 gap-3 text-xs">
          <div><p className="text-muted-foreground">Access</p><p className={value?.hasAccess ? "text-green-600" : "text-destructive"}>{value?.hasAccess ? "Yes" : "No"}</p></div>
          <div><p className="text-muted-foreground">Balance</p><p>{value?.balance ?? "-"}</p></div>
          <div><p className="text-muted-foreground">Usage</p><p>{value?.usage ?? "-"}</p></div>
          <div><p className="text-muted-foreground">Overage</p><p>{value?.overage ?? "-"}</p></div>
        </div>
        {grants && grants.length > 0 && (
          <div className="mt-3">
            <p className="mb-1 text-xs font-medium text-muted-foreground">Grants</p>
            {grants.map((g) => (
              <div key={g.id} className="mb-1 rounded-lg border border-glass-border-honey px-2 py-1 text-xs">
                <span className="text-amber">Amount: {g.amount}</span>
                <span className="ml-2 text-muted-foreground">Priority: {g.priority}</span>
                <span className="ml-2 text-muted-foreground">Effective: {formatDateTime(g.effectiveAt)}</span>
              </div>
            ))}
          </div>
        )}
        <div className="mt-2">
          <Button size="xs" variant="outline" onClick={() => setShowGrant(true)}><Plus className="size-3" /> Add Grant</Button>
          <CreateGrantDialog open={showGrant} onOpenChange={setShowGrant} onSubmit={(body) => createGrant.mutate({ customerId, entitlementId: entitlement.id, body }, { onSuccess: () => setShowGrant(false) })} isPending={createGrant.isPending} />
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function CreateEntitlementDialog({ open, onOpenChange, onSubmit, isPending }: { open: boolean; onOpenChange: (open: boolean) => void; onSubmit: (body: unknown) => void; isPending: boolean }) {
  const { data: spec, isLoading: schemaLoading } = useOpenAPISchema();
  const [form, setForm] = useState({ featureKey: "", type: "metered" });
  const [rawJson, setRawJson] = useState("");
  const [mode, setMode] = useState<string>("pretty");
  const [validated, setValidated] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);

  const handleTabChange = useCallback((tab: string) => {
    if (tab === "json") { setRawJson(JSON.stringify(form, null, 2)); }
    else { try { const p = JSON.parse(rawJson); setForm({ featureKey: p.featureKey ?? "", type: p.type ?? "metered" }); } catch {} }
    setMode(tab); setValidated(false); setErrors([]);
  }, [form, rawJson]);

  const getBody = (): Record<string, unknown> | null => {
    if (mode === "json") { try { return JSON.parse(rawJson); } catch { setErrors(["Invalid JSON"]); return null; } }
    return form;
  };

  const handleValidate = () => {
    const body = getBody();
    if (!body || !spec) return;
    const schema = extractSchema(spec, SCHEMA_REFS.EntitlementCreate);
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
              <DialogTitle>Create Entitlement</DialogTitle>
              <DialogDescription>Grant a feature entitlement to this customer.</DialogDescription>
            </DialogHeader>
            <TabsList className="h-6 p-0.5">
              <TabsTrigger value="pretty" className="h-5 px-2 text-[10px]">Pretty</TabsTrigger>
              <TabsTrigger value="json" className="h-5 px-2 text-[10px]">JSON</TabsTrigger>
            </TabsList>
          </div>
          <TabsContent value="pretty" className="mt-3">
            <FieldGroup className="gap-4">
              <Field className="gap-1.5"><FieldLabel className="text-xs">Feature Key <span className="text-destructive">*</span></FieldLabel><Input className="h-8 text-xs" value={form.featureKey} onChange={(e) => { setForm((f) => ({ ...f, featureKey: e.target.value })); setValidated(false); setErrors([]); }} /></Field>
              <Field className="gap-1.5"><FieldLabel className="text-xs">Type</FieldLabel><Input className="h-8 text-xs" value={form.type} onChange={(e) => { setForm((f) => ({ ...f, type: e.target.value })); setValidated(false); setErrors([]); }} /></Field>
            </FieldGroup>
          </TabsContent>
          <TabsContent value="json" className="mt-3">
            <Textarea value={rawJson} onChange={(e) => { setRawJson(e.target.value); setValidated(false); setErrors([]); }} className="min-h-24 font-mono text-xs" />
          </TabsContent>
        </Tabs>
        {errors.length > 0 && <ul className="text-xs text-destructive space-y-0.5">{errors.map((e, i) => <li key={i}>{e}</li>)}</ul>}
        {validated && <p className="flex items-center gap-1 text-xs text-green-600"><CheckCircle2 className="size-3" /> Validation passed</p>}
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>Cancel</Button>
          {validated ? <Button size="sm" onClick={() => { const b = getBody(); if (b) onSubmit(b); }} disabled={isPending}>Create</Button> : <Button size="sm" variant="secondary" onClick={handleValidate} disabled={schemaLoading}>{schemaLoading ? "Loading..." : "Validate"}</Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function CreateGrantDialog({ open, onOpenChange, onSubmit, isPending }: { open: boolean; onOpenChange: (open: boolean) => void; onSubmit: (body: unknown) => void; isPending: boolean }) {
  const { data: spec, isLoading: schemaLoading } = useOpenAPISchema();
  const [form, setForm] = useState({ amount: "100", priority: "1" });
  const [rawJson, setRawJson] = useState("");
  const [mode, setMode] = useState<string>("pretty");
  const [validated, setValidated] = useState(false);
  const [errors, setErrors] = useState<string[]>([]);

  const handleTabChange = useCallback((tab: string) => {
    if (tab === "json") { setRawJson(JSON.stringify({ amount: Number(form.amount), priority: Number(form.priority), effectiveAt: new Date().toISOString() }, null, 2)); }
    else { try { const p = JSON.parse(rawJson); setForm({ amount: String(p.amount ?? "100"), priority: String(p.priority ?? "1") }); } catch {} }
    setMode(tab); setValidated(false); setErrors([]);
  }, [form, rawJson]);

  const getBody = (): Record<string, unknown> | null => {
    if (mode === "json") { try { return JSON.parse(rawJson); } catch { setErrors(["Invalid JSON"]); return null; } }
    return { amount: Number(form.amount), priority: Number(form.priority), effectiveAt: new Date().toISOString() };
  };

  const handleValidate = () => {
    const body = getBody();
    if (!body || !spec) return;
    const schema = extractSchema(spec, SCHEMA_REFS.GrantCreate);
    const { valid, errors: valErrors } = validateAgainstSchema(schema, body);
    if (valid) { setValidated(true); setErrors([]); }
    else { setValidated(false); setErrors(formatErrors(valErrors)); }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xs" showCloseButton={false}>
        <Tabs value={mode} onValueChange={handleTabChange}>
          <div className="flex items-start justify-between">
            <DialogHeader>
              <DialogTitle>Add Grant</DialogTitle>
              <DialogDescription>Add a usage grant to this entitlement.</DialogDescription>
            </DialogHeader>
            <TabsList className="h-6 p-0.5">
              <TabsTrigger value="pretty" className="h-5 px-2 text-[10px]">Pretty</TabsTrigger>
              <TabsTrigger value="json" className="h-5 px-2 text-[10px]">JSON</TabsTrigger>
            </TabsList>
          </div>
          <TabsContent value="pretty" className="mt-3">
            <FieldGroup className="gap-4">
              <Field className="gap-1.5"><FieldLabel className="text-xs">Amount</FieldLabel><Input className="h-8 text-xs" value={form.amount} onChange={(e) => { setForm((f) => ({ ...f, amount: e.target.value })); setValidated(false); setErrors([]); }} /></Field>
              <Field className="gap-1.5"><FieldLabel className="text-xs">Priority</FieldLabel><Input className="h-8 text-xs" value={form.priority} onChange={(e) => { setForm((f) => ({ ...f, priority: e.target.value })); setValidated(false); setErrors([]); }} /></Field>
            </FieldGroup>
          </TabsContent>
          <TabsContent value="json" className="mt-3">
            <Textarea value={rawJson} onChange={(e) => { setRawJson(e.target.value); setValidated(false); setErrors([]); }} className="min-h-24 font-mono text-xs" />
          </TabsContent>
        </Tabs>
        {errors.length > 0 && <ul className="text-xs text-destructive space-y-0.5">{errors.map((e, i) => <li key={i}>{e}</li>)}</ul>}
        {validated && <p className="flex items-center gap-1 text-xs text-green-600"><CheckCircle2 className="size-3" /> Validation passed</p>}
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>Cancel</Button>
          {validated ? <Button size="sm" onClick={() => { const b = getBody(); if (b) onSubmit(b); }} disabled={isPending}>Add</Button> : <Button size="sm" variant="secondary" onClick={handleValidate} disabled={schemaLoading}>{schemaLoading ? "Loading..." : "Validate"}</Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
