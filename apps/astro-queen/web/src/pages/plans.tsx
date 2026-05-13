import { useMemo, useState } from "react";
import {
  usePlans, useCreatePlan, useUpdatePlan, useDeletePlan, usePublishPlan, useArchivePlan, useFeatures,
  useCustomers, useMigrateSubscription,
} from "@/api/openmeter";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Field, FieldLabel, FieldGroup } from "@/components/ui/field";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@/components/ui/select";
import { Combobox } from "@/components/ui/combobox";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "@/components/ui/collapsible";
import {
  Dialog, DialogTrigger, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter,
} from "@/components/ui/dialog";
import { Trash2, Upload, Archive, ChevronDown, Plus, X, Pencil, ArrowUpCircle } from "lucide-react";
import { cn, formatDateTime } from "@/lib/utils";
import type { Plan, Customer } from "@/types/openmeter";

const STATUS_COLORS: Record<string, string> = {
  draft: "text-yellow-600",
  active: "text-green-600",
  archived: "text-muted-foreground",
  scheduled: "text-blue-500",
};

const BILLING_CADENCES = [
  { value: "P1M", label: "Monthly (P1M)" },
  { value: "P3M", label: "Quarterly (P3M)" },
  { value: "P1Y", label: "Yearly (P1Y)" },
];

// ─── Rate Card form state ───

interface RateCardForm {
  type: string;
  key: string;
  name: string;
  featureKey: string;
  billingCadence: string;
  priceAmount: string;
  entitlementType: string;
  issueAfterReset: string;
  isSoftLimit: boolean;
  staticConfig: string;
}

const EMPTY_RATE_CARD: RateCardForm = {
  type: "flat_fee",
  key: "",
  name: "",
  featureKey: "",
  billingCadence: "",
  priceAmount: "",
  entitlementType: "",
  issueAfterReset: "",
  isSoftLimit: false,
  staticConfig: "{}",
};

// ─── Phase form state ───

interface PhaseForm {
  key: string;
  name: string;
  duration: string;
  rateCards: RateCardForm[];
}

const EMPTY_PHASE: PhaseForm = {
  key: "",
  name: "",
  duration: "",
  rateCards: [{ ...EMPTY_RATE_CARD }],
};

// ─── Plan form state ───

interface PlanForm {
  name: string;
  key: string;
  description: string;
  currency: string;
  billingCadence: string;
  phases: PhaseForm[];
}

const EMPTY_PLAN: PlanForm = {
  name: "",
  key: "",
  description: "",
  currency: "USD",
  billingCadence: "P1M",
  phases: [{ ...EMPTY_PHASE, key: "default", name: "Default" }],
};

// ─── Page ───

export function PlansPage() {
  const { data, isLoading, error } = usePlans();
  const { data: customers } = useCustomers();
  const deleteMut = useDeletePlan();
  const publishMut = usePublishPlan();
  const archiveMut = useArchivePlan();
  const [showCreate, setShowCreate] = useState(false);
  const [editingPlan, setEditingPlan] = useState<Plan | null>(null);

  // Latest active version per plan key
  const latestByKey = useMemo(() => {
    const map = new Map<string, Plan>();
    for (const p of data ?? []) {
      if (p.status !== "active") continue;
      const existing = map.get(p.key);
      if (!existing || p.version > existing.version) map.set(p.key, p);
    }
    return map;
  }, [data]);

  // Customers grouped by `${planKey}::${planVersion}` of their current subscription
  const customersByPlanVersion = useMemo(() => {
    const map = new Map<string, Customer[]>();
    for (const c of customers ?? []) {
      const current = c.subscriptions?.find((s) => s.id === c.currentSubscriptionId);
      if (!current?.plan || current.status !== "active") continue;
      const key = `${current.plan.key}::${current.plan.version}`;
      const list = map.get(key) ?? [];
      list.push(c);
      map.set(key, list);
    }
    return map;
  }, [customers]);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Plans</h2>
          <p className="text-[10px] text-muted-foreground">
            Reusable pricing templates with phases and rate cards. Subscribe customers to auto-provision entitlements.
          </p>
        </div>
        {!showCreate && !editingPlan && (
          <Button size="xs" onClick={() => setShowCreate(true)}>
            <Plus className="size-3 mr-1" /> Create Plan
          </Button>
        )}
      </div>

      {showCreate && <CreatePlanForm onDone={() => setShowCreate(false)} />}
      {editingPlan && (
        <EditPlanForm
          plan={editingPlan}
          onDone={() => setEditingPlan(null)}
        />
      )}

      {isLoading && <Skeleton className="h-40 w-full" />}
      {error && <p className="text-destructive text-sm">{error.message}</p>}
      {data && data.length === 0 && !showCreate && (
        <p className="text-sm text-muted-foreground">No plans found.</p>
      )}
      {data && data.length > 0 && (
        <div className="space-y-2">
          {data.map((plan) => {
            const latest = latestByKey.get(plan.key);
            const subscribers = customersByPlanVersion.get(`${plan.key}::${plan.version}`) ?? [];
            const canMigrate = !!latest && latest.version > plan.version && subscribers.length > 0;
            return (
              <PlanRow
                key={plan.id}
                plan={plan}
                subscribers={subscribers}
                latestVersion={canMigrate ? latest! : undefined}
                onEdit={() => { setShowCreate(false); setEditingPlan(plan); }}
                onDelete={() => { if (confirm(`Delete plan "${plan.name}"?`)) deleteMut.mutate(plan.id); }}
                onPublish={() => { if (confirm(`Publish plan "${plan.name}"? This makes it available for subscriptions.`)) publishMut.mutate(plan.id); }}
                onArchive={() => { if (confirm(`Archive plan "${plan.name}"?`)) archiveMut.mutate(plan.id); }}
              />
            );
          })}
        </div>
      )}
    </div>
  );
}

// ─── Create Plan Form ───

function CreatePlanForm({ onDone }: { onDone: () => void }) {
  const createMut = useCreatePlan();
  const { data: features } = useFeatures();
  const [form, setForm] = useState<PlanForm>({ ...EMPTY_PLAN });
  const [jsonMode, setJsonMode] = useState(false);
  const [rawJson, setRawJson] = useState("");
  const [validated, setValidated] = useState(false);
  const [err, setErr] = useState("");

  const set = <K extends keyof PlanForm>(k: K, v: PlanForm[K]) => {
    setForm((f) => ({ ...f, [k]: v }));
    setValidated(false); setErr("");
  };

  const setPhase = (pi: number, patch: Partial<PhaseForm>) => {
    setForm((f) => ({
      ...f,
      phases: f.phases.map((p, i) => (i === pi ? { ...p, ...patch } : p)),
    }));
    setValidated(false); setErr("");
  };

  const addPhase = () => {
    setForm((f) => ({
      ...f,
      phases: [...f.phases, { ...EMPTY_PHASE, key: `phase_${f.phases.length + 1}`, name: `Phase ${f.phases.length + 1}` }],
    }));
    setValidated(false); setErr("");
  };

  const removePhase = (pi: number) => {
    setForm((f) => ({ ...f, phases: f.phases.filter((_, i) => i !== pi) }));
    setValidated(false); setErr("");
  };

  const setRateCard = (pi: number, ri: number, patch: Partial<RateCardForm>) => {
    setForm((f) => ({
      ...f,
      phases: f.phases.map((p, i) =>
        i === pi
          ? { ...p, rateCards: p.rateCards.map((rc, j) => (j === ri ? { ...rc, ...patch } : rc)) }
          : p
      ),
    }));
    setValidated(false); setErr("");
  };

  const addRateCard = (pi: number) => {
    setForm((f) => ({
      ...f,
      phases: f.phases.map((p, i) =>
        i === pi ? { ...p, rateCards: [...p.rateCards, { ...EMPTY_RATE_CARD }] } : p
      ),
    }));
    setValidated(false); setErr("");
  };

  const removeRateCard = (pi: number, ri: number) => {
    setForm((f) => ({
      ...f,
      phases: f.phases.map((p, i) =>
        i === pi ? { ...p, rateCards: p.rateCards.filter((_, j) => j !== ri) } : p
      ),
    }));
    setValidated(false); setErr("");
  };

  const buildBody = (): Record<string, unknown> | null => {
    if (jsonMode) {
      try { return JSON.parse(rawJson); }
      catch { setErr("Invalid JSON"); return null; }
    }

    if (!form.name || !form.key || !form.billingCadence) {
      setErr("Name, key, and billing cadence are required.");
      return null;
    }
    if (form.phases.length === 0) {
      setErr("At least one phase is required.");
      return null;
    }
    for (const phase of form.phases) {
      if (!phase.key || !phase.name) {
        setErr(`Phase "${phase.name || phase.key || "?"}" is missing a name or key.`);
        return null;
      }
      for (const rc of phase.rateCards) {
        if (!rc.key || !rc.name) {
          setErr(`Rate card in phase "${phase.name}" is missing a key or name.`);
          return null;
        }
        if (rc.entitlementType && !rc.featureKey) {
          setErr(`Rate card "${rc.name}" has a ${rc.entitlementType} entitlement but no feature. Set a feature or remove the entitlement.`);
          return null;
        }
      }
    }

    const phases = form.phases.map((phase) => {
      const rateCards = phase.rateCards.map((rc) => {
        const card: Record<string, unknown> = {
          type: rc.type,
          key: rc.key,
          name: rc.name,
          billingCadence: rc.type === "flat_fee" ? (rc.billingCadence || null) : rc.billingCadence,
          price: rc.priceAmount
            ? { type: "flat", amount: rc.priceAmount, paymentTerm: "in_arrears" }
            : null,
        };
        if (rc.featureKey) card.featureKey = rc.featureKey;
        if (rc.entitlementType) {
          const ent: Record<string, unknown> = { type: rc.entitlementType };
          if (rc.entitlementType === "metered") {
            if (rc.issueAfterReset) ent.issueAfterReset = Number(rc.issueAfterReset);
            ent.isSoftLimit = rc.isSoftLimit;
          }
          if (rc.entitlementType === "static") {
            ent.config = rc.staticConfig || "{}";
          }
          card.entitlementTemplate = ent;
        }
        return card;
      });

      return {
        key: phase.key,
        name: phase.name,
        duration: phase.duration || null,
        rateCards,
      };
    });

    return {
      name: form.name,
      key: form.key,
      currency: form.currency,
      billingCadence: form.billingCadence,
      ...(form.description ? { description: form.description } : {}),
      phases,
    };
  };

  const handleValidate = () => {
    setErr("");
    const body = buildBody();
    if (body) { setValidated(true); setErr(""); }
  };

  const handleSubmit = () => {
    setErr("");
    const body = buildBody();
    if (!body) return;
    createMut.mutate(body, {
      onSuccess: () => onDone(),
      onError: (e) => { setErr(e.message); setValidated(false); },
    });
  };

  const switchToJson = () => {
    setErr(""); setValidated(false);
    const body = buildBody();
    setRawJson(JSON.stringify(body ?? {}, null, 2));
    setJsonMode(true);
  };

  const parseJsonToForm = (json: string): PlanForm | null => {
    try {
      const obj = JSON.parse(json);
      const phases: PhaseForm[] = (obj.phases ?? []).map((phase: Record<string, unknown>) => {
        const rateCards: RateCardForm[] = ((phase.rateCards as Record<string, unknown>[]) ?? []).map((rc) => {
          const ent = rc.entitlementTemplate as Record<string, unknown> | undefined;
          return {
            type: (rc.type as string) || "flat_fee",
            key: (rc.key as string) || "",
            name: (rc.name as string) || "",
            featureKey: (rc.featureKey as string) || "",
            billingCadence: (rc.billingCadence as string) || "",
            priceAmount: rc.price && typeof rc.price === "object" ? String((rc.price as Record<string, unknown>).amount ?? "") : "",
            entitlementType: ent ? (ent.type as string) || "" : "",
            issueAfterReset: ent?.issueAfterReset != null ? String(ent.issueAfterReset) : "",
            isSoftLimit: ent?.isSoftLimit === true,
            staticConfig: ent?.type === "static" ? (ent.config as string) || "{}" : "{}",
          };
        });
        return {
          key: (phase.key as string) || "",
          name: (phase.name as string) || "",
          duration: (phase.duration as string) || "",
          rateCards: rateCards.length > 0 ? rateCards : [{ ...EMPTY_RATE_CARD }],
        };
      });
      return {
        name: obj.name || "",
        key: obj.key || "",
        description: obj.description || "",
        currency: obj.currency || "USD",
        billingCadence: obj.billingCadence || "P1M",
        phases: phases.length > 0 ? phases : [{ ...EMPTY_PHASE, key: "default", name: "Default" }],
      };
    } catch {
      return null;
    }
  };

  const switchToPretty = () => {
    const parsed = parseJsonToForm(rawJson);
    if (parsed) {
      setForm(parsed);
    }
    setJsonMode(false);
    setErr(""); setValidated(false);
  };

  return (
    <div className="rounded-lg glass p-4 space-y-3">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold">Create Plan</h3>
          <p className="text-[10px] text-muted-foreground">
            A plan is a reusable pricing template. It is created as a <strong className="text-foreground">draft</strong>, then published to make it available for customer subscriptions.
          </p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <Button variant="ghost" size="xs" onClick={jsonMode ? switchToPretty : switchToJson}>
            {jsonMode ? "Pretty" : "JSON"}
          </Button>
          <Button variant="ghost" size="icon-xs" onClick={onDone} title="Close">
            <X className="size-3.5" />
          </Button>
        </div>
      </div>

      {jsonMode ? (
        <textarea
          value={rawJson}
          onChange={(e) => { setRawJson(e.target.value); setErr(""); setValidated(false); }}
          className="w-full min-h-48 rounded border border-glass-border-honey bg-transparent px-2 py-1.5 font-mono text-[10px] focus:outline-none focus:ring-1 focus:ring-amber"
        />
      ) : (
        <>
          {/* Plan basics */}
          <FieldGroup className="gap-2">
            <div className="grid grid-cols-4 gap-2">
              <Field className="gap-1">
                <FieldLabel className="text-[10px]">Name *</FieldLabel>
                <Input value={form.name} onChange={(e) => set("name", e.target.value)} placeholder="Pro Plan" />
                <p className="text-[9px] text-muted-foreground">Display name shown to users</p>
              </Field>
              <Field className="gap-1">
                <FieldLabel className="text-[10px]">Key *</FieldLabel>
                <Input value={form.key} onChange={(e) => set("key", e.target.value)} placeholder="pro_plan" />
                <p className="text-[9px] text-muted-foreground">Unique ID (lowercase, underscores)</p>
              </Field>
              <Field className="gap-1">
                <FieldLabel className="text-[10px]">Currency</FieldLabel>
                <Input value={form.currency} onChange={(e) => set("currency", e.target.value)} placeholder="USD" />
                <p className="text-[9px] text-muted-foreground">ISO 4217 code</p>
              </Field>
              <Field className="gap-1">
                <FieldLabel className="text-[10px]">Billing Cadence *</FieldLabel>
                <Select value={form.billingCadence} onValueChange={(v) => set("billingCadence", v)}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {BILLING_CADENCES.map((c) => (
                      <SelectItem key={c.value} value={c.value}>{c.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-[9px] text-muted-foreground">How often customers are billed</p>
              </Field>
            </div>
            <Field className="gap-1">
              <FieldLabel className="text-[10px]">Description</FieldLabel>
              <Input value={form.description} onChange={(e) => set("description", e.target.value)} placeholder="Optional description for this plan" />
            </Field>
          </FieldGroup>

          {/* Phases */}
          <div className="space-y-3">
            <div>
              <span className="text-[11px] font-semibold">Phases</span>
              <p className="text-[9px] text-muted-foreground">
                Phases let you change pricing over time (e.g. a trial phase followed by a paid phase). Each phase has its own rate cards. A phase switch happens at the end of a billing period. Most plans need just one phase.
              </p>
            </div>
            {form.phases.map((phase, pi) => (
              <PhaseEditor
                key={pi}
                phase={phase}
                index={pi}
                canRemove={form.phases.length > 1}
                features={features ?? []}
                onChange={(patch) => setPhase(pi, patch)}
                onRemove={() => removePhase(pi)}
                onRateCardChange={(ri, patch) => setRateCard(pi, ri, patch)}
                onAddRateCard={() => addRateCard(pi)}
                onRemoveRateCard={(ri) => removeRateCard(pi, ri)}
              />
            ))}
            <Button variant="outline" size="xs" onClick={addPhase}>
              <Plus className="size-3 mr-1" /> Add Phase
            </Button>
          </div>
        </>
      )}

      {/* Live preview */}
      {!jsonMode && form.phases.some((p) => p.rateCards.some((rc) => rc.name)) && (
        <div className="rounded border border-amber/30 bg-pollen/10 p-2.5 space-y-1.5">
          <span className="text-[10px] font-semibold">Preview: what subscribers will get</span>
          <p className="text-[9px] text-muted-foreground">
            When a customer subscribes to <strong className="text-foreground">{form.name || "this plan"}</strong>, these entitlements are auto-created:
          </p>
          <div className="overflow-x-auto rounded border border-glass-border-honey">
            <table className="w-full text-[9px] whitespace-nowrap">
              <thead>
                <tr className="glass-subtle">
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Phase</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Rate Card</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Feature</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Entitlement</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Price</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Grant / Config</th>
                </tr>
              </thead>
              <tbody className="text-muted-foreground">
                {form.phases.flatMap((phase, pi) =>
                  phase.rateCards.filter((rc) => rc.name || rc.key).map((rc, ri) => (
                    <tr key={`${pi}-${ri}`} className="border-t border-glass-border-honey">
                      <td className="px-2 py-0.5">{phase.name || `Phase ${pi + 1}`}</td>
                      <td className="px-2 py-0.5 text-foreground">{rc.name || rc.key || "—"}</td>
                      <td className="px-2 py-0.5 font-mono text-amber">{rc.featureKey || "—"}</td>
                      <td className="px-2 py-0.5">
                        {rc.entitlementType ? (
                          <span className={
                            rc.entitlementType === "metered" ? "text-blue-500" :
                            rc.entitlementType === "boolean" ? "text-green-600" :
                            rc.entitlementType === "static" ? "text-purple-500" : ""
                          }>
                            {rc.entitlementType}
                          </span>
                        ) : (
                          <span className="italic">charge only</span>
                        )}
                      </td>
                      <td className="px-2 py-0.5">
                        {rc.priceAmount ? `${form.currency} ${rc.priceAmount}/${rc.billingCadence || form.billingCadence || "period"}` : "free"}
                      </td>
                      <td className="px-2 py-0.5">
                        {rc.entitlementType === "metered" && rc.issueAfterReset
                          ? `${rc.issueAfterReset} credits/period${rc.isSoftLimit ? " (soft)" : ""}`
                          : rc.entitlementType === "static"
                          ? <span className="font-mono">{rc.staticConfig || "{}"}</span>
                          : rc.entitlementType === "boolean"
                          ? "on/off"
                          : "—"}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
          {(() => {
            const entCount = form.phases.flatMap((p) => p.rateCards).filter((rc) => rc.entitlementType).length;
            const chargeOnly = form.phases.flatMap((p) => p.rateCards).filter((rc) => (rc.name || rc.key) && !rc.entitlementType).length;
            return (
              <p className="text-[9px] text-muted-foreground">
                {entCount > 0 && <><strong className="text-foreground">{entCount}</strong> entitlement{entCount !== 1 ? "s" : ""} created</>}
                {entCount > 0 && chargeOnly > 0 && " + "}
                {chargeOnly > 0 && <><strong className="text-foreground">{chargeOnly}</strong> charge-only rate card{chargeOnly !== 1 ? "s" : ""}</>}
                {entCount === 0 && chargeOnly === 0 && "Fill in rate cards above to see the preview."}
              </p>
            );
          })()}
        </div>
      )}

      {/* Example */}
      {!jsonMode && (
        <details className="group rounded border border-glass-border-honey">
          <summary className="cursor-pointer px-2.5 py-1.5 text-[10px] font-medium text-muted-foreground hover:text-foreground select-none">
            Example: how rate cards become entitlements
          </summary>
          <div className="px-2.5 pb-2 pt-1 space-y-1.5">
            <p className="text-[9px] text-muted-foreground">
              Each rate card in a plan creates one independent entitlement per customer when they subscribe. Think of it as: <strong className="text-foreground">1 rate card = 1 feature = 1 entitlement</strong>.
            </p>
            <div className="overflow-x-auto rounded border border-glass-border-honey">
              <table className="w-full text-[9px] whitespace-nowrap">
                <thead>
                  <tr className="glass-subtle">
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Rate Card</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Feature</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Type</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Price</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Entitlement created</th>
                    <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Grant</th>
                  </tr>
                </thead>
                <tbody className="text-muted-foreground">
                  <tr className="border-t border-glass-border-honey">
                    <td className="px-2 py-0.5 text-foreground">API Access</td>
                    <td className="px-2 py-0.5 font-mono text-amber">api_calls</td>
                    <td className="px-2 py-0.5 text-blue-500">metered</td>
                    <td className="px-2 py-0.5">$49/mo</td>
                    <td className="px-2 py-0.5">Tracks usage against balance</td>
                    <td className="px-2 py-0.5">10,000 credits/period</td>
                  </tr>
                  <tr className="border-t border-glass-border-honey">
                    <td className="px-2 py-0.5 text-foreground">Storage</td>
                    <td className="px-2 py-0.5 font-mono text-amber">storage_gb</td>
                    <td className="px-2 py-0.5 text-blue-500">metered</td>
                    <td className="px-2 py-0.5">$0.10/GB</td>
                    <td className="px-2 py-0.5">Tracks usage against balance</td>
                    <td className="px-2 py-0.5">50 GB/period</td>
                  </tr>
                  <tr className="border-t border-glass-border-honey">
                    <td className="px-2 py-0.5 text-foreground">Pro Features</td>
                    <td className="px-2 py-0.5 font-mono text-amber">pro_features</td>
                    <td className="px-2 py-0.5 text-green-600">boolean</td>
                    <td className="px-2 py-0.5">included</td>
                    <td className="px-2 py-0.5">On/off access</td>
                    <td className="px-2 py-0.5">-</td>
                  </tr>
                  <tr className="border-t border-glass-border-honey">
                    <td className="px-2 py-0.5 text-foreground">Integration Tier</td>
                    <td className="px-2 py-0.5 font-mono text-amber">integrations</td>
                    <td className="px-2 py-0.5 text-purple-500">static</td>
                    <td className="px-2 py-0.5">included</td>
                    <td className="px-2 py-0.5">Config: {`{"github":true,"slack":true}`}</td>
                    <td className="px-2 py-0.5">-</td>
                  </tr>
                  <tr className="border-t border-glass-border-honey">
                    <td className="px-2 py-0.5 text-foreground">Platform Fee</td>
                    <td className="px-2 py-0.5">-</td>
                    <td className="px-2 py-0.5">none</td>
                    <td className="px-2 py-0.5">$10/mo</td>
                    <td className="px-2 py-0.5 italic">No entitlement (charge only)</td>
                    <td className="px-2 py-0.5">-</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p className="text-[9px] text-muted-foreground">
              When a customer subscribes to this plan, they get <strong className="text-foreground">4 independent entitlements</strong> (one per rate card with a feature). The "Platform Fee" has no feature, so it's just a charge with no entitlement. Each entitlement is checked separately in your app.
            </p>
          </div>
        </details>
      )}

      {err && <p className="text-[10px] text-destructive">{err}</p>}
      {createMut.error && <p className="text-[10px] text-destructive">{createMut.error.message}</p>}

      <div className="flex justify-end gap-2">
        <Button variant="outline" size="xs" onClick={onDone}>Cancel</Button>
        {validated ? (
          <Button
            size="xs"
            onClick={handleSubmit}
            disabled={createMut.isPending}
            className="bg-green-600 hover:bg-green-700 text-white"
          >
            Create Plan
          </Button>
        ) : (
          <Button size="xs" onClick={handleValidate}>
            Validate
          </Button>
        )}
      </div>
    </div>
  );
}

// ─── Plan → Form converter ───

function planToForm(plan: Plan): PlanForm {
  return {
    name: plan.name,
    key: plan.key,
    description: plan.description || "",
    currency: plan.currency,
    billingCadence: plan.billingCadence,
    phases: plan.phases.map((phase) => ({
      key: phase.key,
      name: phase.name,
      duration: phase.duration || "",
      rateCards: phase.rateCards.map((rc) => {
        const ent = rc.entitlementTemplate as Record<string, unknown> | undefined;
        return {
          type: rc.type || "flat_fee",
          key: rc.key,
          name: rc.name,
          featureKey: rc.featureKey || "",
          billingCadence: rc.billingCadence || "",
          priceAmount: rc.price && typeof rc.price === "object" ? String((rc.price as Record<string, unknown>).amount ?? "") : "",
          entitlementType: ent ? String(ent.type ?? "") : "",
          issueAfterReset: ent?.issueAfterReset != null ? String(ent.issueAfterReset) : "",
          isSoftLimit: ent?.isSoftLimit === true,
          staticConfig: ent?.type === "static" ? String(ent.config ?? "{}") : "{}",
        };
      }),
    })),
  };
}

// ─── Edit Plan Form ───

function EditPlanForm({ plan, onDone }: { plan: Plan; onDone: () => void }) {
  const updateMut = useUpdatePlan();
  const { data: features } = useFeatures();
  const [form, setForm] = useState<PlanForm>(() => planToForm(plan));
  const [jsonMode, setJsonMode] = useState(false);
  const [rawJson, setRawJson] = useState("");
  const [err, setErr] = useState("");

  const set = <K extends keyof PlanForm>(k: K, v: PlanForm[K]) => {
    setForm((f) => ({ ...f, [k]: v }));
    setErr("");
  };

  const setPhase = (pi: number, patch: Partial<PhaseForm>) => {
    setForm((f) => ({
      ...f,
      phases: f.phases.map((p, i) => (i === pi ? { ...p, ...patch } : p)),
    }));
    setErr("");
  };

  const addPhase = () => {
    setForm((f) => ({
      ...f,
      phases: [...f.phases, { ...EMPTY_PHASE, key: `phase_${f.phases.length + 1}`, name: `Phase ${f.phases.length + 1}` }],
    }));
    setErr("");
  };

  const removePhase = (pi: number) => {
    setForm((f) => ({ ...f, phases: f.phases.filter((_, i) => i !== pi) }));
    setErr("");
  };

  const setRateCard = (pi: number, ri: number, patch: Partial<RateCardForm>) => {
    setForm((f) => ({
      ...f,
      phases: f.phases.map((p, i) =>
        i === pi
          ? { ...p, rateCards: p.rateCards.map((rc, j) => (j === ri ? { ...rc, ...patch } : rc)) }
          : p
      ),
    }));
    setErr("");
  };

  const addRateCard = (pi: number) => {
    setForm((f) => ({
      ...f,
      phases: f.phases.map((p, i) =>
        i === pi ? { ...p, rateCards: [...p.rateCards, { ...EMPTY_RATE_CARD }] } : p
      ),
    }));
    setErr("");
  };

  const removeRateCard = (pi: number, ri: number) => {
    setForm((f) => ({
      ...f,
      phases: f.phases.map((p, i) =>
        i === pi ? { ...p, rateCards: p.rateCards.filter((_, j) => j !== ri) } : p
      ),
    }));
    setErr("");
  };

  const buildBody = (): Record<string, unknown> | null => {
    if (jsonMode) {
      try { return JSON.parse(rawJson); }
      catch { setErr("Invalid JSON"); return null; }
    }

    if (!form.name || !form.billingCadence) {
      setErr("Name and billing cadence are required.");
      return null;
    }

    const phases = form.phases.map((phase) => {
      const rateCards = phase.rateCards.map((rc) => {
        const card: Record<string, unknown> = {
          type: rc.type,
          key: rc.key,
          name: rc.name,
          billingCadence: rc.type === "flat_fee" ? (rc.billingCadence || null) : rc.billingCadence,
          price: rc.priceAmount
            ? { type: "flat", amount: rc.priceAmount, paymentTerm: "in_arrears" }
            : null,
        };
        if (rc.featureKey) card.featureKey = rc.featureKey;
        if (rc.entitlementType) {
          const ent: Record<string, unknown> = { type: rc.entitlementType };
          if (rc.entitlementType === "metered") {
            if (rc.issueAfterReset) ent.issueAfterReset = Number(rc.issueAfterReset);
            ent.isSoftLimit = rc.isSoftLimit;
          }
          if (rc.entitlementType === "static") {
            ent.config = rc.staticConfig || "{}";
          }
          card.entitlementTemplate = ent;
        }
        return card;
      });

      return {
        key: phase.key,
        name: phase.name,
        duration: phase.duration || null,
        rateCards,
      };
    });

    return {
      name: form.name,
      currency: form.currency,
      billingCadence: form.billingCadence,
      ...(form.description ? { description: form.description } : {}),
      phases,
    };
  };

  const handleSubmit = () => {
    setErr("");
    const body = buildBody();
    if (!body) return;
    updateMut.mutate(
      { id: plan.id, body },
      {
        onSuccess: () => onDone(),
        onError: (e) => setErr(e.message),
      }
    );
  };

  const switchToJson = () => {
    setErr("");
    const body = buildBody();
    setRawJson(JSON.stringify(body ?? {}, null, 2));
    setJsonMode(true);
  };

  const switchToPretty = () => {
    try {
      const obj = JSON.parse(rawJson);
      const parsed = planToForm({ ...plan, ...obj, phases: obj.phases ?? plan.phases });
      setForm(parsed);
    } catch { /* keep current form */ }
    setJsonMode(false);
    setErr("");
  };

  return (
    <div className="rounded-lg glass p-4 space-y-3">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold">Edit Plan: {plan.name}</h3>
          <p className="text-[10px] text-muted-foreground">
            <span className="font-mono text-amber">{plan.key}</span> &middot; v{plan.version} &middot; {plan.status}
          </p>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <Button variant="ghost" size="xs" onClick={jsonMode ? switchToPretty : switchToJson}>
            {jsonMode ? "Pretty" : "JSON"}
          </Button>
          <Button variant="ghost" size="icon-xs" onClick={onDone} title="Close">
            <X className="size-3.5" />
          </Button>
        </div>
      </div>

      {jsonMode ? (
        <textarea
          value={rawJson}
          onChange={(e) => { setRawJson(e.target.value); setErr(""); }}
          className="w-full min-h-48 rounded border border-glass-border-honey bg-transparent px-2 py-1.5 font-mono text-[10px] focus:outline-none focus:ring-1 focus:ring-amber"
        />
      ) : (
        <>
          <FieldGroup className="gap-2">
            <div className="grid grid-cols-4 gap-2">
              <Field className="gap-1">
                <FieldLabel className="text-[10px]">Name *</FieldLabel>
                <Input value={form.name} onChange={(e) => set("name", e.target.value)} />
              </Field>
              <Field className="gap-1">
                <FieldLabel className="text-[10px]">Key</FieldLabel>
                <Input value={form.key} disabled className="opacity-50" />
              </Field>
              <Field className="gap-1">
                <FieldLabel className="text-[10px]">Currency</FieldLabel>
                <Input value={form.currency} onChange={(e) => set("currency", e.target.value)} />
              </Field>
              <Field className="gap-1">
                <FieldLabel className="text-[10px]">Billing Cadence *</FieldLabel>
                <Select value={form.billingCadence} onValueChange={(v) => set("billingCadence", v)}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {BILLING_CADENCES.map((c) => (
                      <SelectItem key={c.value} value={c.value}>{c.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </div>
            <Field className="gap-1">
              <FieldLabel className="text-[10px]">Description</FieldLabel>
              <Input value={form.description} onChange={(e) => set("description", e.target.value)} />
            </Field>
          </FieldGroup>

          <div className="space-y-3">
            <span className="text-[11px] font-semibold">Phases</span>
            {form.phases.map((phase, pi) => (
              <PhaseEditor
                key={pi}
                phase={phase}
                index={pi}
                canRemove={form.phases.length > 1}
                features={features ?? []}
                onChange={(patch) => setPhase(pi, patch)}
                onRemove={() => removePhase(pi)}
                onRateCardChange={(ri, patch) => setRateCard(pi, ri, patch)}
                onAddRateCard={() => addRateCard(pi)}
                onRemoveRateCard={(ri) => removeRateCard(pi, ri)}
              />
            ))}
            <Button variant="outline" size="xs" onClick={addPhase}>
              <Plus className="size-3 mr-1" /> Add Phase
            </Button>
          </div>
        </>
      )}

      {err && <p className="text-[10px] text-destructive">{err}</p>}
      {updateMut.error && <p className="text-[10px] text-destructive">{updateMut.error.message}</p>}

      <div className="flex justify-end gap-2">
        <Button variant="outline" size="xs" onClick={onDone}>Cancel</Button>
        <Button
          size="xs"
          onClick={handleSubmit}
          disabled={updateMut.isPending}
        >
          {updateMut.isPending ? "Saving..." : "Save Changes"}
        </Button>
      </div>
    </div>
  );
}

// ─── Phase Editor ───

function PhaseEditor({
  phase,
  index,
  canRemove,
  features,
  onChange,
  onRemove,
  onRateCardChange,
  onAddRateCard,
  onRemoveRateCard,
}: {
  phase: PhaseForm;
  index: number;
  canRemove: boolean;
  features: { key: string; name: string }[];
  onChange: (patch: Partial<PhaseForm>) => void;
  onRemove: () => void;
  onRateCardChange: (ri: number, patch: Partial<RateCardForm>) => void;
  onAddRateCard: () => void;
  onRemoveRateCard: (ri: number) => void;
}) {
  return (
    <div className="rounded border border-glass-border-honey p-2.5 space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-[11px] font-semibold">Phase {index + 1}</span>
        {canRemove && (
          <Button variant="ghost" size="icon-xs" onClick={onRemove} title="Remove phase">
            <Trash2 className="size-3 text-red-500" />
          </Button>
        )}
      </div>
      <div className="grid grid-cols-3 gap-2">
        <Field className="gap-1">
          <FieldLabel className="text-[10px]">Name *</FieldLabel>
          <Input value={phase.name} onChange={(e) => onChange({ name: e.target.value })} placeholder="Default" />
        </Field>
        <Field className="gap-1">
          <FieldLabel className="text-[10px]">Key *</FieldLabel>
          <Input value={phase.key} onChange={(e) => onChange({ key: e.target.value })} placeholder="default" />
        </Field>
        <Field className="gap-1">
          <FieldLabel className="text-[10px]">Duration</FieldLabel>
          <Input value={phase.duration} onChange={(e) => onChange({ duration: e.target.value })} placeholder="e.g. P1Y (empty = forever)" />
          <p className="text-[9px] text-muted-foreground">ISO8601 duration. Leave empty for the last/only phase.</p>
        </Field>
      </div>

      <div className="space-y-2">
        <div>
          <span className="text-[10px] font-medium text-muted-foreground">Rate Cards</span>
          <p className="text-[9px] text-muted-foreground">
            Each rate card defines a line item: what feature it gates, how it's priced, and what entitlement the customer gets when subscribed.
          </p>
        </div>
        {phase.rateCards.map((rc, ri) => (
          <RateCardEditor
            key={ri}
            rc={rc}
            index={ri}
            features={features}
            canRemove={phase.rateCards.length > 1}
            onChange={(patch) => onRateCardChange(ri, patch)}
            onRemove={() => onRemoveRateCard(ri)}
          />
        ))}
        <Button variant="outline" size="xs" onClick={onAddRateCard}>
          <Plus className="size-3 mr-1" /> Add Rate Card
        </Button>
      </div>
    </div>
  );
}

// ─── Rate Card Editor ───

function RateCardEditor({
  rc,
  index,
  features,
  canRemove,
  onChange,
  onRemove,
}: {
  rc: RateCardForm;
  index: number;
  features: { key: string; name: string }[];
  canRemove: boolean;
  onChange: (patch: Partial<RateCardForm>) => void;
  onRemove: () => void;
}) {
  return (
    <div className="rounded border border-dashed border-glass-border-honey p-2 space-y-1.5">
      <div className="flex items-center justify-between">
        <span className="text-[10px] font-medium text-muted-foreground">Rate Card {index + 1}</span>
        {canRemove && (
          <Button variant="ghost" size="icon-xs" onClick={onRemove} title="Remove rate card">
            <X className="size-2.5 text-red-500" />
          </Button>
        )}
      </div>
      <div className="grid grid-cols-5 gap-2">
        <Field className="gap-1">
          <FieldLabel className="text-[10px]">Type *</FieldLabel>
          <Select value={rc.type} onValueChange={(v) => onChange({ type: v })}>
            <SelectTrigger><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="flat_fee">Flat Fee</SelectItem>
              <SelectItem value="usage_based">Usage Based</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-[9px] text-muted-foreground">{rc.type === "flat_fee" ? "Fixed recurring or one-time charge" : "Price based on metered usage"}</p>
        </Field>
        <Field className="gap-1">
          <FieldLabel className="text-[10px]">Key *</FieldLabel>
          <Input value={rc.key} onChange={(e) => onChange({ key: e.target.value })} placeholder="api_access" />
        </Field>
        <Field className="gap-1">
          <FieldLabel className="text-[10px]">Name *</FieldLabel>
          <Input value={rc.name} onChange={(e) => onChange({ name: e.target.value })} placeholder="API Access" />
        </Field>
        <Field className="gap-1">
          <FieldLabel className="text-[10px]">Feature *</FieldLabel>
          <Combobox
            options={(features ?? []).map((f) => ({ value: f.key, label: f.name, description: f.key }))}
            value={rc.featureKey}
            onValueChange={(v) => {
              onChange(v ? { featureKey: v } : { featureKey: "", entitlementType: "", issueAfterReset: "", isSoftLimit: false, staticConfig: "{}" });
            }}
            placeholder="Select feature..."
            searchPlaceholder="Search features..."
            emptyText="No features found."
          />
        </Field>
        <Field className="gap-1">
          <FieldLabel className="text-[10px]">Price (flat)</FieldLabel>
          <Input value={rc.priceAmount} onChange={(e) => onChange({ priceAmount: e.target.value })} placeholder="0 = free" />
          <p className="text-[9px] text-muted-foreground">Leave empty for free tier</p>
        </Field>
      </div>

      {/* Billing cadence */}
      <Field className="gap-1">
        <FieldLabel className="text-[10px]">Billing cadence override</FieldLabel>
        <Select value={rc.billingCadence || "_null"} onValueChange={(v) => onChange({ billingCadence: v === "_null" ? "" : v })}>
          <SelectTrigger className="w-48"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="_null">{rc.type === "flat_fee" ? "One-time" : "Inherit from plan"}</SelectItem>
            {BILLING_CADENCES.map((c) => (
              <SelectItem key={c.value} value={c.value}>{c.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>

      {/* Entitlement type picker */}
      <div className="space-y-1.5">
        <FieldLabel className="text-[10px]">Entitlement type</FieldLabel>
        <p className="text-[9px] text-muted-foreground">
          {rc.featureKey
            ? <>What access does the customer get for <strong className="text-amber font-mono">{rc.featureKey}</strong>?</>
            : "What access does the customer get? Requires a feature to be set."}
        </p>
        <div className="grid grid-cols-4 gap-1.5">
          {([
            { value: "", label: "None", desc: "No entitlement. Charge only — no feature gating.", color: "text-muted-foreground" },
            { value: "metered", label: "Metered", desc: "Usage tracked against a balance. Grants replenish credits each period.", color: "text-blue-500" },
            { value: "boolean", label: "Boolean", desc: "Simple on/off access. Customer either has the feature or doesn't.", color: "text-green-600" },
            { value: "static", label: "Static", desc: "Access with a JSON config. Use for feature flags and tier configs.", color: "text-purple-500" },
          ] as const).map((opt) => (
            <button
              key={opt.value}
              type="button"
              onClick={() => onChange({ entitlementType: opt.value })}
              className={cn(
                "rounded border p-1.5 text-left transition-all",
                rc.entitlementType === opt.value
                  ? "border-amber bg-pollen/30 ring-1 ring-amber/50"
                  : "border-glass-border-honey hover:bg-glass-light"
              )}
            >
              <span className={`text-[10px] font-semibold ${opt.color}`}>{opt.label}</span>
              <p className="text-[8px] text-muted-foreground leading-tight mt-0.5">{opt.desc}</p>
            </button>
          ))}
        </div>

        {/* Metered settings */}
        {rc.entitlementType === "metered" && (
          <div className="rounded border border-blue-500/20 bg-blue-50/5 p-2 space-y-1.5 mt-1">
            <span className="text-[10px] font-medium text-blue-500">Metered entitlement settings</span>
            <div className="grid grid-cols-3 gap-2">
              <Field className="gap-1">
                <FieldLabel className="text-[10px]">Grant after reset</FieldLabel>
                <Input
                  type="number"
                  value={rc.issueAfterReset}
                  onChange={(e) => onChange({ issueAfterReset: e.target.value })}
                  placeholder="e.g. 10000"
                />
                <p className="text-[9px] text-muted-foreground">Credits auto-issued each billing period. This creates a recurring grant.</p>
              </Field>
              <Field className="gap-1">
                <FieldLabel className="text-[10px]">Soft limit</FieldLabel>
                <Select value={rc.isSoftLimit ? "true" : "false"} onValueChange={(v) => onChange({ isSoftLimit: v === "true" })}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="false">No &mdash; hard limit</SelectItem>
                    <SelectItem value="true">Yes &mdash; soft limit</SelectItem>
                  </SelectContent>
                </Select>
                <p className="text-[9px] text-muted-foreground">Hard: blocks access at 0 balance. Soft: allows overage.</p>
              </Field>
            </div>
          </div>
        )}

        {/* Static settings */}
        {rc.entitlementType === "static" && (
          <div className="rounded border border-purple-500/20 bg-purple-50/5 p-2 space-y-1 mt-1">
            <span className="text-[10px] font-medium text-purple-500">Static entitlement config</span>
            <Field className="gap-1">
              <Input
                value={rc.staticConfig}
                onChange={(e) => onChange({ staticConfig: e.target.value })}
                placeholder='e.g. {"integrations":["github","slack"]}'
                className="font-mono"
              />
              <p className="text-[9px] text-muted-foreground">JSON object returned when checking access. Use to encode tier-specific feature flags.</p>
            </Field>
          </div>
        )}
      </div>
    </div>
  );
}

// ─── Plan Row (existing list) ───

function PlanRow({
  plan,
  subscribers,
  latestVersion,
  onEdit,
  onDelete,
  onPublish,
  onArchive,
}: {
  plan: Plan;
  subscribers: Customer[];
  latestVersion?: Plan;
  onEdit: () => void;
  onDelete: () => void;
  onPublish: () => void;
  onArchive: () => void;
}) {
  return (
    <Collapsible className="rounded-lg glass">
      <div className="flex items-center justify-between px-3 py-2">
        <CollapsibleTrigger className="flex items-center gap-2 text-sm hover:text-foreground min-w-0">
          <ChevronDown className="size-3.5 text-muted-foreground shrink-0" />
          <span className="font-mono text-xs text-amber">{plan.key}</span>
          <span className="font-medium truncate">{plan.name}</span>
          <span className={`text-[10px] ${STATUS_COLORS[plan.status] || "text-muted-foreground"}`}>
            {plan.status}
          </span>
          <span className="text-[10px] text-muted-foreground">v{plan.version}</span>
        </CollapsibleTrigger>
        <div className="flex items-center gap-1 shrink-0">
          <span className="text-[10px] text-muted-foreground mr-2">
            {plan.currency} &middot; {plan.billingCadence}
          </span>
          {latestVersion && (
            <MigrateDialog
              plan={plan}
              latestVersion={latestVersion}
              subscribers={subscribers}
            />
          )}
          {(plan.status === "draft" || plan.status === "scheduled") && (
            <Button variant="ghost" size="icon-xs" title="Edit" onClick={onEdit}>
              <Pencil className="size-3" />
            </Button>
          )}
          {plan.status === "draft" && (
            <Button variant="ghost" size="icon-xs" title="Publish" onClick={onPublish}>
              <Upload className="size-3 text-green-600" />
            </Button>
          )}
          {plan.status === "active" && (
            <Button variant="ghost" size="icon-xs" title="Archive" onClick={onArchive}>
              <Archive className="size-3 text-amber" />
            </Button>
          )}
          {plan.status === "draft" && (
            <Button variant="ghost" size="icon-xs" title="Delete" onClick={onDelete}>
              <Trash2 className="size-3 text-red-500" />
            </Button>
          )}
        </div>
      </div>
      <CollapsibleContent className="border-t border-glass-border-honey px-3 py-2">
        {plan.description && (
          <p className="text-[10px] text-muted-foreground mb-2">{plan.description}</p>
        )}
        <p className="text-[10px] text-muted-foreground mb-1">
          Created {formatDateTime(plan.createdAt)}
        </p>
        {plan.phases.map((phase, i) => (
          <div key={phase.key} className="mt-2">
            <div className="flex items-center gap-2 mb-1">
              <span className="text-[10px] font-medium">
                Phase {i + 1}: {phase.name}
              </span>
              <span className="text-[10px] text-muted-foreground">
                ({phase.key})
              </span>
              {phase.duration && (
                <span className="text-[10px] text-muted-foreground">
                  Duration: {phase.duration}
                </span>
              )}
            </div>
            {phase.rateCards.length > 0 && (
              <div className="overflow-x-auto rounded border border-glass-border-honey">
                <table className="w-full text-[10px] whitespace-nowrap">
                  <thead>
                    <tr className="glass-subtle">
                      <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Type</th>
                      <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Key</th>
                      <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Name</th>
                      <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Feature</th>
                      <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Entitlement</th>
                      <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Limit</th>
                      <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Price</th>
                    </tr>
                  </thead>
                  <tbody>
                    {phase.rateCards.map((rc) => {
                      const ent = rc.entitlementTemplate as Record<string, unknown> | undefined;
                      const entType = ent?.type as string | undefined;
                      const price = rc.price as Record<string, unknown> | undefined;
                      return (
                        <tr key={rc.key} className="border-t border-glass-border-honey">
                          <td className="px-2 py-0.5">
                            <span className={rc.type === "flat_fee" ? "text-purple-500" : "text-blue-500"}>
                              {rc.type}
                            </span>
                          </td>
                          <td className="px-2 py-0.5 font-mono text-amber">{rc.key}</td>
                          <td className="px-2 py-0.5">{rc.name}</td>
                          <td className="px-2 py-0.5 text-muted-foreground">{rc.featureKey || "-"}</td>
                          <td className="px-2 py-0.5">
                            {entType ? (
                              <span className={
                                entType === "metered" ? "text-blue-500" :
                                entType === "boolean" ? "text-green-600" :
                                entType === "static" ? "text-purple-500" : ""
                              }>
                                {entType}{entType === "metered" && ent?.isSoftLimit === false ? " (hard)" : entType === "metered" ? " (soft)" : ""}
                              </span>
                            ) : (
                              <span className="text-muted-foreground italic">none</span>
                            )}
                          </td>
                          <td className="px-2 py-0.5 text-muted-foreground">
                            {entType === "metered" && ent?.issueAfterReset != null
                              ? String(ent.issueAfterReset)
                              : entType === "static" && ent?.config
                              ? <span className="font-mono">{String(ent.config)}</span>
                              : "-"}
                          </td>
                          <td className="px-2 py-0.5 text-muted-foreground">
                            {price ? `${price.amount ?? "?"}/${rc.billingCadence || plan.billingCadence}` : "free"}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
            {phase.rateCards.length === 0 && (
              <p className="text-[10px] text-muted-foreground ml-2">No rate cards</p>
            )}
          </div>
        ))}
      </CollapsibleContent>
    </Collapsible>
  );
}

// ─── Migrate Dialog ───

function MigrateDialog({
  plan,
  latestVersion,
  subscribers,
}: {
  plan: Plan;
  latestVersion: Plan;
  subscribers: Customer[];
}) {
  const migrateMut = useMigrateSubscription();
  const [open, setOpen] = useState(false);
  const [running, setRunning] = useState(false);
  const [progress, setProgress] = useState(0);
  const [errors, setErrors] = useState<{ customerId: string; message: string }[]>([]);
  const [done, setDone] = useState(false);

  const run = async () => {
    setRunning(true);
    setProgress(0);
    setErrors([]);
    setDone(false);

    for (let i = 0; i < subscribers.length; i++) {
      const c = subscribers[i];
      const subId = c.currentSubscriptionId;
      if (!subId) {
        setProgress(i + 1);
        continue;
      }
      try {
        await migrateMut.mutateAsync({ id: subId, targetVersion: latestVersion.version });
      } catch (e) {
        setErrors((prev) => [...prev, { customerId: c.id, message: (e as Error).message }]);
      }
      setProgress(i + 1);
    }

    setRunning(false);
    setDone(true);
  };

  return (
    <Dialog open={open} onOpenChange={(o) => {
      setOpen(o);
      if (!o) { setDone(false); setProgress(0); setErrors([]); }
    }}>
      <DialogTrigger asChild>
        <Button
          variant="ghost"
          size="xs"
          title={`Migrate ${subscribers.length} subscriber${subscribers.length !== 1 ? "s" : ""} to v${latestVersion.version}`}
        >
          <ArrowUpCircle className="size-3 text-amber mr-1" />
          Migrate {subscribers.length} &rarr; v{latestVersion.version}
        </Button>
      </DialogTrigger>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Migrate subscribers to v{latestVersion.version}</DialogTitle>
          <DialogDescription>
            Move all customers on <span className="font-mono text-amber">{plan.key}</span> v{plan.version} to v{latestVersion.version}. Each subscription is migrated individually via OpenMeter&apos;s <span className="font-mono text-[10px]">/migrate</span> endpoint &mdash; no bulk API exists, so we iterate.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-2">
          <div className="flex items-center justify-between text-[11px]">
            <span>
              <strong>{subscribers.length}</strong> customer{subscribers.length !== 1 ? "s" : ""} on v{plan.version}
            </span>
            {running && (
              <span className="text-muted-foreground">{progress} / {subscribers.length}</span>
            )}
            {done && !running && (
              <span className={errors.length > 0 ? "text-amber" : "text-green-600"}>
                {errors.length > 0 ? `${progress - errors.length} migrated, ${errors.length} failed` : `All ${progress} migrated`}
              </span>
            )}
          </div>

          <div className="max-h-60 overflow-y-auto rounded border border-glass-border-honey">
            <table className="w-full text-[11px]">
              <thead className="glass-subtle sticky top-0">
                <tr>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Customer</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">ID</th>
                  <th className="px-2 py-0.5 text-left font-medium text-muted-foreground">Status</th>
                </tr>
              </thead>
              <tbody>
                {subscribers.map((c) => {
                  const err = errors.find((e) => e.customerId === c.id);
                  return (
                    <tr key={c.id} className="border-t border-glass-border-honey">
                      <td className="px-2 py-0.5">{c.name || "-"}</td>
                      <td className="px-2 py-0.5 font-mono text-[10px] text-muted-foreground">{c.id}</td>
                      <td className="px-2 py-0.5">
                        {err ? (
                          <span className="text-red-500" title={err.message}>failed</span>
                        ) : done ? (
                          <span className="text-green-600">migrated</span>
                        ) : (
                          <span className="text-muted-foreground">pending</span>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {errors.length > 0 && (
            <details className="text-[10px]">
              <summary className="cursor-pointer text-muted-foreground">View errors ({errors.length})</summary>
              <ul className="mt-1 space-y-0.5">
                {errors.map((e) => (
                  <li key={e.customerId} className="text-red-500">
                    <span className="font-mono">{e.customerId}</span>: {e.message}
                  </li>
                ))}
              </ul>
            </details>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" size="xs" onClick={() => setOpen(false)} disabled={running}>
            {done ? "Close" : "Cancel"}
          </Button>
          {!done && (
            <Button size="xs" onClick={run} disabled={running || subscribers.length === 0}>
              {running ? `Migrating ${progress}/${subscribers.length}...` : `Migrate ${subscribers.length}`}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
